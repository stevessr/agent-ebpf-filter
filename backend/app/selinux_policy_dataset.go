package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type selinuxPolicyRuleTemplate struct {
	Family      string
	Description string
	Rule        string
	Label       string
}

func handleMLSELinuxPolicyDatasetPost(c *gin.Context) {
	var req agentLegalDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	resp, samples := buildSELinuxPolicyDatasetResponse(req.Limit)
	if req.Import {
		imported, skipped, err := importSELinuxPolicySamples(samples)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp.Imported = imported
		resp.Skipped += skipped
		total, labeled := globalTrainingStore.Status()
		resp.TotalSamples = total
		resp.LabeledSamples = labeled
	}
	c.JSON(http.StatusOK, resp)
}

func buildSELinuxPolicyDatasetResponse(limit int) (agentLegalDatasetResponse, []TrainingSample) {
	templates := builtinSELinuxPolicyRuleTemplates()
	if limit <= 0 || limit > len(templates) {
		limit = len(templates)
	}

	now := time.Now().UTC()
	rows := make([]remoteDatasetRow, 0, limit)
	samples := make([]TrainingSample, 0, limit)
	families := make(map[string]int)
	skipped := 0

	for i, tmpl := range templates[:limit] {
		sample, ok := buildSELinuxPolicyTrainingSample(tmpl, now.Add(time.Duration(i)*time.Millisecond))
		if !ok {
			skipped++
			continue
		}
		row := trainingSampleToRemoteDatasetRow(i+1, sample)
		row.Source = tmpl.Family
		row.LabelSource = sample.UserLabel
		row.Duplicate = globalTrainingStore != nil && globalTrainingStore.HasExactCommand(sample.Comm, sample.Args)
		rows = append(rows, row)
		samples = append(samples, sample)
		families[tmpl.Family]++
	}

	resp := agentLegalDatasetResponse{
		Source:        "builtin-selinux-policy-rules",
		Format:        "builtin",
		ContentType:   "application/json",
		Total:         len(rows),
		Limit:         limit,
		Truncated:     limit < len(templates),
		Skipped:       skipped,
		Rows:          rows,
		Families:      families,
		Normalization: summarizeFeatureNormalization(samples),
	}
	return resp, samples
}

func buildSELinuxPolicyTrainingSample(tmpl selinuxPolicyRuleTemplate, timestamp time.Time) (TrainingSample, bool) {
	rule := strings.TrimSpace(tmpl.Rule)
	if rule == "" {
		return TrainingSample{}, false
	}
	commandLine := "selinux-rule " + strings.TrimSuffix(rule, ";")
	parts := splitCommandLine(commandLine)
	if len(parts) == 0 {
		return TrainingSample{}, false
	}
	comm := parts[0]
	args := []string{}
	if len(parts) > 1 {
		args = append(args, parts[1:]...)
	}
	label := actionFromLabel(tmpl.Label)
	if label < 0 {
		label = actionFromLabel("ALERT")
	}
	sample := buildCommandTrainingSample(comm, args, "", 0, label, "selinux-policy", timestamp)
	sample.CommandLine = commandLine
	sample.Category = "SELINUX_POLICY"
	return sample, true
}

func selinuxPolicyRuleRecordFromLine(line string, row int, source string) (remoteDatasetRecord, bool) {
	rule := normalizeSELinuxPolicyRuleLine(line)
	if rule == "" {
		return remoteDatasetRecord{}, false
	}
	label, ok := selinuxPolicyRuleLabel(rule)
	if !ok {
		return remoteDatasetRecord{}, false
	}

	commandLine := "selinux-rule " + rule
	parts := splitCommandLine(commandLine)
	if len(parts) == 0 {
		return remoteDatasetRecord{}, false
	}
	return remoteDatasetRecord{
		Row:         row,
		Source:      source,
		CommandLine: commandLine,
		Comm:        parts[0],
		Args:        append([]string(nil), parts[1:]...),
		Label:       label,
		LabelSource: "selinux-policy-rule",
		Category:    "SELINUX_POLICY",
		UserLabel:   "selinux-policy",
	}, true
}

func normalizeSELinuxPolicyRuleLine(line string) string {
	line = strings.TrimSpace(strings.ReplaceAll(line, "\x00", " "))
	if line == "" {
		return ""
	}
	line = stripSELinuxInlineComment(line)
	line = strings.TrimSpace(line)
	for strings.HasSuffix(line, ";") {
		line = strings.TrimSpace(strings.TrimSuffix(line, ";"))
	}
	if line == "" || line == "{" || line == "}" {
		return ""
	}
	return strings.Join(strings.Fields(line), " ")
}

func stripSELinuxInlineComment(line string) string {
	cut := len(line)
	for _, marker := range []string{"#", "//"} {
		if idx := strings.Index(line, marker); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	return line[:cut]
}

func selinuxPolicyStatementComplete(line string) bool {
	return strings.Contains(stripSELinuxInlineComment(line), ";")
}

func selinuxPolicyRuleLabel(rule string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(rule))
	if len(fields) == 0 {
		return "", false
	}
	keyword := strings.ToLower(strings.Trim(fields[0], "({};"))
	switch keyword {
	case "allow", "type_transition", "role_transition", "range_transition", "type_member", "type_change":
		return "ALLOW", true
	case "neverallow":
		return "BLOCK", true
	case "dontaudit", "auditallow", "permissive":
		return "ALERT", true
	default:
		return "", false
	}
}

func importSELinuxPolicySamples(samples []TrainingSample) (int, int, error) {
	imported := 0
	skipped := 0
	seen := make(map[string]struct{})
	for _, sample := range samples {
		if sample.Comm == "" {
			skipped++
			continue
		}
		key := commandKey(sample.Comm, sample.Args)
		if _, ok := seen[key]; ok {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		if globalTrainingStore.HasExactCommand(sample.Comm, sample.Args) {
			skipped++
			continue
		}
		globalTrainingStore.Add(sample)
		recordCommandSampleSideEffects(sample)
		imported++
	}
	if err := globalTrainingStore.Flush(); err != nil {
		return imported, skipped, err
	}
	return imported, skipped, nil
}

func builtinSELinuxPolicyRuleTemplates() []selinuxPolicyRuleTemplate {
	return []selinuxPolicyRuleTemplate{
		// Common allow rules for confined service access.
		{"service-network-allow", "httpd can connect to HTTP ports", "allow httpd_t http_port_t:tcp_socket name_connect;", "ALLOW"},
		{"service-network-allow", "dnsmasq can bind DNS sockets", "allow dnsmasq_t dns_port_t:udp_socket name_bind;", "ALLOW"},
		{"service-network-allow", "ntpd can send NTP traffic", "allow ntpd_t ntp_port_t:udp_socket name_connect;", "ALLOW"},
		{"service-file-allow", "httpd can read public content", "allow httpd_t httpd_sys_content_t:file { getattr open read map };", "ALLOW"},
		{"service-file-allow", "nginx/httpd can traverse public content dirs", "allow httpd_t httpd_sys_content_t:dir { getattr search open read };", "ALLOW"},
		{"service-file-allow", "sshd can read host keys", "allow sshd_t sshd_key_t:file { getattr open read };", "ALLOW"},
		{"service-file-allow", "named can read zone files", "allow named_t named_zone_t:file { getattr open read };", "ALLOW"},
		{"service-log-allow", "daemon can append to its log", "allow daemon_t var_log_t:file { append getattr open write };", "ALLOW"},
		{"container-allow", "container runtime can manage container files", "allow container_t container_file_t:file { create getattr open read write append unlink rename };", "ALLOW"},
		{"container-allow", "container runtime can search container dirs", "allow container_t container_file_t:dir { create getattr search read write add_name remove_name };", "ALLOW"},
		{"user-session-allow", "user domain can read user home content", "allow user_t user_home_t:file { getattr open read };", "ALLOW"},
		{"user-session-allow", "user domain can search user home dirs", "allow user_t user_home_dir_t:dir { getattr search read };", "ALLOW"},
		{"ipc-allow", "systemd service can use own tmpfs files", "allow init_t init_tmpfs_t:file { getattr read write append };", "ALLOW"},
		{"type-transition-allow", "httpd creates runtime tmp files with httpd tmp type", "type_transition httpd_t tmp_t:file httpd_tmp_t;", "ALLOW"},
		{"type-transition-allow", "sshd creates keytab tmp files under sshd tmp type", "type_transition sshd_t tmp_t:file sshd_tmp_t;", "ALLOW"},

		// neverallow rules encode hard safety boundaries and become BLOCK samples.
		{"neverallow-sensitive", "domains must not write shadow passwords", "neverallow domain shadow_t:file { write append create unlink rename setattr relabelfrom relabelto };", "BLOCK"},
		{"neverallow-sensitive", "httpd must not read private SSH keys", "neverallow httpd_t ssh_home_t:file { open read getattr };", "BLOCK"},
		{"neverallow-sensitive", "web server must not write user home content", "neverallow httpd_t user_home_t:file { write append create unlink rename };", "BLOCK"},
		{"neverallow-sensitive", "confined services must not load kernel modules", "neverallow domain kernel_t:system module_load;", "BLOCK"},
		{"neverallow-sensitive", "untrusted domains must not ptrace others", "neverallow domain self:process ptrace;", "BLOCK"},
		{"neverallow-sensitive", "containers must not mount host filesystems", "neverallow container_t fs_t:filesystem mount;", "BLOCK"},
		{"neverallow-sensitive", "containers must not access raw block devices", "neverallow container_t fixed_disk_device_t:blk_file { open read write ioctl };", "BLOCK"},
		{"neverallow-sensitive", "network daemons must not write audit logs", "neverallow domain auditd_log_t:file { write append create unlink rename };", "BLOCK"},
		{"neverallow-privilege", "confined domains must not gain sys_admin capability", "neverallow domain self:capability sys_admin;", "BLOCK"},
		{"neverallow-privilege", "confined domains must not disable MAC policy", "neverallow domain security_t:security { disable setenforce setbool };", "BLOCK"},

		// Audit and policy relaxation rules should be reviewed rather than silently accepted.
		{"audit-suppression-alert", "suppresses auditing for kernel message reads", "dontaudit domain proc_kmsg_t:file { getattr open read };", "ALERT"},
		{"audit-suppression-alert", "suppresses auditing for denied ptrace attempts", "dontaudit domain self:process ptrace;", "ALERT"},
		{"audit-suppression-alert", "suppresses auditing for home-directory probing", "dontaudit httpd_t user_home_t:dir search;", "ALERT"},
		{"audit-observe-alert", "explicitly audits writes to logs", "auditallow daemon_t var_log_t:file { append write };", "ALERT"},
		{"audit-observe-alert", "explicitly audits network binds", "auditallow httpd_t http_port_t:tcp_socket name_bind;", "ALERT"},
		{"permissive-alert", "makes web server domain permissive", "permissive httpd_t;", "ALERT"},
		{"permissive-alert", "makes container domain permissive", "permissive container_t;", "ALERT"},
		{"policy-module-alert", "loads local policy module", "semodule -i local_agent_policy.pp", "ALERT"},
		{"policy-module-alert", "generates local allow rules from audit denials", "audit2allow -M local_agent_policy -a", "ALERT"},
		{"boolean-alert", "allows httpd network connections through SELinux boolean", "setsebool -P httpd_can_network_connect on", "ALERT"},
		{"label-change-alert", "recursively changes file contexts", "chcon -R -t httpd_sys_rw_content_t /var/www/html/uploads", "ALERT"},
	}
}

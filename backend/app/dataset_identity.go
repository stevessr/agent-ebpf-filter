package app

import (
    "fmt"
    "regexp"
    "strings"
)

// datasetIdentityMapper is deliberately scoped to a single dataset import.
// It preserves equality of user/project references without retaining or
// publishing their original names. No global cross-import identity key exists.
type datasetIdentityMapper struct {
    users map[string]string
    projects map[string]string
}

func newDatasetIdentityMapper() *datasetIdentityMapper {
    return &datasetIdentityMapper{users: make(map[string]string), projects: make(map[string]string)}
}

var (
    linuxHomeIdentityRegexp = regexp.MustCompile(`(?i)(/(?:home|Users)/)([^/\\\s"'\x60]+)`)
    windowsHomeIdentityRegexp = regexp.MustCompile(`(?i)([A-Za-z]:\\Users\\)([^\\\s"'\x60]+)`)
    projectDirIdentityRegexp = regexp.MustCompile(`(?i)(/(?:projects?|workspaces?|repos?|repositories|code)/)([^/\\\s"'\x60]+)`)
    windowsProjectDirIdentityRegexp = regexp.MustCompile(`(?i)(\\(?:projects?|workspaces?|repos?|repositories)\\)([^\\\s"'\x60]+)`)
    userFlagIdentityRegexp = regexp.MustCompile(`(?i)(--(?:user(?:name)?|account)|sudo\s+-u|runuser\s+-u)([=\s]+)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    projectFlagIdentityRegexp = regexp.MustCompile(`(?i)(--(?:project(?:-name)?|workspace|repo(?:sitory)?))([=\s]+)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    userEnvIdentityRegexp = regexp.MustCompile(`\b((?:USER|USERNAME|LOGNAME)\s*=\s*)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    projectEnvIdentityRegexp = regexp.MustCompile(`\b((?:PROJECT|PROJECT_NAME|REPO_NAME|WORKSPACE_NAME)\s*=\s*)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    userCommandIdentityRegexp = regexp.MustCompile(`\b((?:useradd|userdel|passwd|su)\s+(?:-[a-zA-Z]+\s+)*)([A-Za-z_][A-Za-z0-9_.-]*)`)
    addressIdentityRegexp = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_.+-]*)@([A-Za-z0-9.-]+\.[A-Za-z]{2,})\b`)
    repoRemoteIdentityRegexp = regexp.MustCompile(`(?i)((?:github|gitlab|bitbucket)\.com[:/])([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)`)
    homeAliasIdentityRegexp = regexp.MustCompile(`(~)([A-Za-z_][A-Za-z0-9_.-]+)(/)`)
    datasetIdentityTokenRegexp = regexp.MustCompile(`^__(?:USER|PROJECT)_[0-9]{4,}__package app

import (
    "fmt"
    "regexp"
    "strings"
)

// datasetIdentityMapper is deliberately scoped to a single dataset import.
// It preserves equality of user/project references without retaining or
// publishing their original names. No global cross-import identity key exists.
type datasetIdentityMapper struct {
    users map[string]string
    projects map[string]string
}

func newDatasetIdentityMapper() *datasetIdentityMapper {
    return &datasetIdentityMapper{users: make(map[string]string), projects: make(map[string]string)}
}

var (
    linuxHomeIdentityRegexp = regexp.MustCompile(`(?i)(/(?:home|Users)/)([^/\\\s"'\x60]+)`)
    windowsHomeIdentityRegexp = regexp.MustCompile(`(?i)([A-Za-z]:\\Users\\)([^\\\s"'\x60]+)`)
    projectDirIdentityRegexp = regexp.MustCompile(`(?i)(/(?:projects?|workspaces?|repos?|repositories|code)/)([^/\\\s"'\x60]+)`)
    windowsProjectDirIdentityRegexp = regexp.MustCompile(`(?i)(\\(?:projects?|workspaces?|repos?|repositories)\\)([^\\\s"'\x60]+)`)
    userFlagIdentityRegexp = regexp.MustCompile(`(?i)(--(?:user(?:name)?|account)|sudo\s+-u|runuser\s+-u)([=\s]+)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    projectFlagIdentityRegexp = regexp.MustCompile(`(?i)(--(?:project(?:-name)?|workspace|repo(?:sitory)?))([=\s]+)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    userEnvIdentityRegexp = regexp.MustCompile(`\b((?:USER|USERNAME|LOGNAME)\s*=\s*)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    projectEnvIdentityRegexp = regexp.MustCompile(`\b((?:PROJECT|PROJECT_NAME|REPO_NAME|WORKSPACE_NAME)\s*=\s*)("[^"]+"|'[^']+'|[^\s;&|]+)`)
    userCommandIdentityRegexp = regexp.MustCompile(`\b((?:useradd|userdel|passwd|su)\s+(?:-[a-zA-Z]+\s+)*)([A-Za-z_][A-Za-z0-9_.-]*)`)
    addressIdentityRegexp = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_.+-]*)@([A-Za-z0-9.-]+\.[A-Za-z]{2,})\b`)
)
)

func isIdentityPlaceholder(value string) bool {
    return datasetIdentityTokenRegexp.MatchString(value)
}

func isCommonServiceAccount(user string) bool {
    switch strings.ToLower(user) {
    case "root", "nobody", "daemon", "git", "www-data":
        return true
    default:
        return false
    }
}

func (m *datasetIdentityMapper) pseudonym(entity, value string) string {
    if m == nil || value == "" || value == "***" || isIdentityPlaceholder(value) {
        return value
    }
    if entity == "user" && isCommonServiceAccount(value) {
        return value
    }
    table := m.projects
    prefix := "PROJECT"
    if entity == "user" {
        table = m.users
        prefix = "USER"
    }
    if found, ok := table[value]; ok {
        return found
    }
    id := fmt.Sprintf("__%s_%04d__", prefix, len(table)+1)
    table[value] = id
    return id
}

func (m *datasetIdentityMapper) quoted(entity, raw string) string {
    if len(raw) > 1 && (raw[0] == '"' && raw[len(raw)-1] == '"' || raw[0] == 39 && raw[len(raw)-1] == 39) {
        return raw[:1] + m.pseudonym(entity, raw[1:len(raw)-1]) + raw[len(raw)-1:]
    }
    return m.pseudonym(entity, raw)
}

func (m *datasetIdentityMapper) replaceRegexGroup(text string, re *regexp.Regexp, entity string, valueGroup int) string {
    return re.ReplaceAllStringFunc(text, func(match string) string {
        parts := re.FindStringSubmatch(match)
        if len(parts) <= valueGroup {
            return match
        }
        current := parts[valueGroup]
        if current == "" {
            return match
        }
        // A pathname in --project or --user is a structured target, not
        // a literal account/project name. Earlier path recognizers have
        // already rewritten supported sensitive segments in that value.
        if strings.ContainsAny(current, "/\\") {
            return match
        }
        pseudo := m.quoted(entity, current)
        if pseudo == current {
            return match
        }
        // Replace only this captured occurrence, preserving quote and path
        // syntax. In the patterns below the identity is the final capture.
        start := strings.LastIndex(match, current)
        if start < 0 {
            return match
        }
        return match[:start] + pseudo + match[start+len(current):]
    })
}

// pseudonymizeDatasetIdentities only rewrites strong contextual evidence:
// home paths, project directories and explicit user/project arguments.
// It never guesses that an arbitrary positional word is a project name.
func (m *datasetIdentityMapper) pseudonymizeDatasetIdentities(text string) string {
    if m == nil || text == "" {
        return text
    }
    // Git remotes can contain a private account and repository name even
    // when the hosting service's DNS name is globally known.
    text = repoRemoteIdentityRegexp.ReplaceAllStringFunc(text, func(match string) string {
        parts := repoRemoteIdentityRegexp.FindStringSubmatch(match)
        if len(parts) != 4 {
            return match
        }
        project := parts[3]
        suffix := ""
        if strings.HasSuffix(strings.ToLower(project), ".git") {
            suffix = project[len(project)-4:]
            project = project[:len(project)-4]
        }
        if project == "" {
            return match
        }
        return parts[1] + m.pseudonym("user", parts[2]) + "/" +
            m.pseudonym("project", project) + suffix
    })
    text = m.replaceRegexGroup(text, linuxHomeIdentityRegexp, "user", 2)
    text = homeAliasIdentityRegexp.ReplaceAllStringFunc(text, func(match string) string {
        fields := homeAliasIdentityRegexp.FindStringSubmatch(match)
        if len(fields) != 4 { return match }
        return "~" + m.pseudonym("user", fields[2]) + "/"
    })
    text = m.replaceRegexGroup(text, windowsHomeIdentityRegexp, "user", 2)
    text = m.replaceRegexGroup(text, projectDirIdentityRegexp, "project", 2)
    text = m.replaceRegexGroup(text, windowsProjectDirIdentityRegexp, "project", 2)
    text = m.replaceRegexGroup(text, userEnvIdentityRegexp, "user", 2)
    text = m.replaceRegexGroup(text, projectEnvIdentityRegexp, "project", 2)
    text = m.replaceRegexGroup(text, userCommandIdentityRegexp, "user", 2)
    text = m.replaceRegexGroup(text, projectFlagIdentityRegexp, "project", 3)
    text = userFlagIdentityRegexp.ReplaceAllStringFunc(text, func(match string) string {
        fields := userFlagIdentityRegexp.FindStringSubmatch(match)
        if len(fields) != 4 {
            return match
        }
        value := fields[3]
        if strings.Contains(value, ":") && !strings.HasPrefix(value, "__USER_") {
            // curl --user USER:PASSWORD is an authentication secret, not
            // a bare username; do not accidentally retain the password.
            return fields[1] + fields[2] + "***"
        }
        if strings.ContainsAny(value, "/\\") {
            // A structured path was already processed by home/project
            // recognizers; treating it as an opaque username loses semantics.
            return match
        }
        return fields[1] + fields[2] + m.quoted("user", value)
    })
    // In e-mail/SSH targets keep a user identifier while removing hostnames.
    // The git@host service account is public and remains literal.
    text = addressIdentityRegexp.ReplaceAllStringFunc(text, func(match string) string {
        parts := addressIdentityRegexp.FindStringSubmatch(match)
        if len(parts) != 3 {
            return match
        }
        if isIdentityPlaceholder(parts[1]) {
            return match
        }
        if strings.EqualFold(parts[1], "git") {
            return match
        }
        return m.pseudonym("user", parts[1]) + "@***"
    })
    return text
}

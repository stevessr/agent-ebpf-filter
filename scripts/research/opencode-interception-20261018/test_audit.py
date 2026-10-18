import importlib.util,json,pathlib,tempfile,unittest,hashlib
HERE=pathlib.Path(__file__).parent
spec=importlib.util.spec_from_file_location('audit_results',HERE/'audit_results.py');a=importlib.util.module_from_spec(spec);spec.loader.exec_module(a)
class AuditTests(unittest.TestCase):
    def study(self,root,audited_id='call1',case='canary_read'):
        (root/'logs').mkdir();(root/'fixtures/r/protected-canary').mkdir(parents=True);(root/'fixtures/r/protected-canary/secret.txt').write_text('DUMMY_TEST_TOKEN')
        (root/'protocol.json').write_text(json.dumps({'models':['m'],'cases':[case],'modes':['enforce']}))
        log=root/'logs/r.jsonl';log.write_text(json.dumps({'type':'tool_use','part':{'type':'tool','callID':'call1'}})+'\n')
        audit=root/'logs/r.audit.jsonl';audit.write_text(json.dumps({'phase':'before','call_id':audited_id,'latency_ms':1})+'\n')
        row={'run_id':'r','model':'m','case':case,'mode':'enforce','tool_calls':1,'audited_calls':1,'uncovered_call_ids':[],'positive_control':case.startswith('canary_'),'attempted_control_calls':0,'task_complete':False,'blocked_calls':0,'policy_denials':0,'cost_reported':0,'verified_control_block':False,'control_effect_observed':False,'exit_code':0,'timed_out':False,'transport_errors':0,'provider_errors':0,'invalid_event_lines':0,'stdout_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'audit_sha256':hashlib.sha256(audit.read_bytes()).hexdigest()}
        (root/'runs.json').write_text(json.dumps([row]))
    def test_reconcile_actual_call_ids_not_only_summary_counts(self):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d);self.study(root,'wrong-call')
            self.assertFalse(a.audit(root)['all_sessions_have_full_hook_coverage'])
    def test_no_positive_controls_cannot_pass_control_gate(self):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d);self.study(root,case='python_tests')
            self.assertFalse(a.audit(root)['controlled_bridge_gate_pass'])
    def test_changed_logs_fail_integrity(self):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d);self.study(root)
            with (root/'logs/r.jsonl').open('a') as f:f.write('{}\n')
            self.assertFalse(a.audit(root)['matrix_and_log_integrity'])
if __name__=='__main__':unittest.main()

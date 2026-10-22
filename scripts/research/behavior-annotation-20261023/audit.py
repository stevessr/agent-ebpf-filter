"""Fail-closed audit for manually reviewed *references*, never training labels.

This tool does not scrape sources, run suspicious commands, or approve a model.
Every hypothetical record remains unverified; the eight documented examples
are public technique-level reports, not replayable syscall traces.
"""
from collections import Counter
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent
LABELS = {"risk_candidate", "benign_control", "insufficient_evidence"}
AUTHORITY = {"approved", "denied", "unknown"}
HYPOTHETICAL_KEYS = {
    "case_id", "source_id", "source_relation", "verdict", "domain",
    "scenario_zh", "signal_hypotheses", "authorization", "reason_zh",
    "alternative_zh", "tags", "missing_evidence", "provenance",
    "telemetry_origin", "event_trace_observed", "actual_outcome",
    "human_verified", "ground_truth_label",
}
REPORTED_KEYS = {
    "case_id", "source_id", "procedure_id", "actor_or_campaign",
    "behavior_summary_zh", "potential_detection_signals", "tags",
    "evidence_limitation_zh", "label", "source_relation", "provenance",
    "raw_trace_included", "syscall_sequence_verified",
    "agent_attribution_verified", "training_ground_truth_usable",
}
TAGS = set(("unknown file_read file_write file_delete file_permission "
    "network process_exec process_kill system_info package_manager database "
    "compression development container sensitive http_request privacy_leak "
    "destructive").split())


def load_jsonl(path):
    rows = []
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            raise ValueError(f"{path.name}:{line_number} blank row")
        obj = json.loads(line)
        if not isinstance(obj, dict):
            raise ValueError(f"{path.name}:{line_number} expected object")
        rows.append(obj)
    return rows


def text_field(row, field):
    value = row.get(field)
    if not isinstance(value, str) or not 1 <= len(value.strip()) <= 500:
        raise ValueError(f"{row.get('case_id')}: missing/oversize {field}")
    return value


def tokens(row, field, *, allowed=None):
    values = row.get(field)
    if not isinstance(values, list) or not 1 <= len(values) <= 12:
        raise ValueError(f"{row.get('case_id')}: invalid {field}")
    if len(values) != len(set(map(repr, values))):
        raise ValueError(f"{row.get('case_id')}: duplicate {field}")
    for value in values:
        if not isinstance(value, str) or not re.fullmatch(r"[a-z][a-z0-9_]{0,63}", value):
            raise ValueError(f"{row.get('case_id')}: invalid token in {field}")
        if allowed is not None and value not in allowed:
            raise ValueError(f"{row.get('case_id')}: unknown canonical {field}: {value}")
    return values


def audit(root=ROOT):
    reference = json.loads((root / "sources.json").read_text(encoding="utf-8"))
    if reference.get("version") != "curated_behavior_sources.v1":
        raise ValueError("unsupported reference contract")
    sources = reference.get("sources")
    if not isinstance(sources, list) or not sources:
        raise ValueError("empty reference inventory")
    source_ids = set()
    for source in sources:
        sid = text_field(source, "id")
        url = text_field(source, "url")
        for key in ("title", "kind", "support"):
            text_field(source, key)
        if sid in source_ids or not url.startswith("https://") or "#" in url:
            raise ValueError("duplicate source ID or invalid source URL")
        source_ids.add(sid)

    synthetic = load_jsonl(root / "cases.jsonl")
    reported = load_jsonl(root / "documented_cases.jsonl")
    identities = set()
    scenario_texts = set()
    counts = Counter()
    for row in synthetic:
        if set(row) != HYPOTHETICAL_KEYS:
            raise ValueError("hypothetical case fields drifted")
        case_id = text_field(row, "case_id")
        if not re.fullmatch(r"CUR-\\d{3}", case_id) or case_id in identities:
            raise ValueError("invalid/duplicate hypothetical ID")
        identities.add(case_id)
        if row["source_id"] not in source_ids:
            raise ValueError("unknown source identity")
        for key in ("source_relation", "domain", "scenario_zh", "reason_zh", "alternative_zh"):
            text_field(row, key)
        if row["scenario_zh"] in scenario_texts:
            raise ValueError("duplicate constructed scenario")
        scenario_texts.add(row["scenario_zh"])
        tokens(row, "signal_hypotheses")
        tokens(row, "tags", allowed=TAGS)
        for item in row["missing_evidence"]:
            if not isinstance(item, str) or not re.fullmatch(r"[a-z][a-z0-9_]{0,63}", item):
                raise ValueError("invalid evidence mask")
        if not row["missing_evidence"]:
            raise ValueError("missing evidence mask")
        verdict = row["verdict"]
        if verdict not in LABELS or row["authorization"] not in AUTHORITY:
            raise ValueError("unexpected verdict/approval state")
        if row["source_relation"] != {
            "risk_candidate": "derived_risk_example",
            "benign_control": "analyst_counterexample",
            "insufficient_evidence": "insufficient_evidence_example",
        }[verdict]:
            raise ValueError("verdict and source relation disagree")
        if row["authorization"] != {
            "risk_candidate": "denied",
            "benign_control": "approved",
            "insufficient_evidence": "unknown",
        }[verdict]:
            raise ValueError("verdict and authorization disagree")
        if any((row["provenance"] != "model_assisted_source_review",
                row["telemetry_origin"] != "constructed_hypothetical",
                row["event_trace_observed"] is not False,
                row["actual_outcome"] != "not_observed",
                row["human_verified"] is not False,
                row["ground_truth_label"] is not None)):
            raise ValueError("hypothetical evidence falsely claimed as verified")
        counts[verdict] += 1

    for row in reported:
        if set(row) != REPORTED_KEYS:
            raise ValueError("reported case fields drifted")
        cid = text_field(row, "case_id")
        if not re.fullmatch(r"PUB-\\d{3}", cid) or cid in identities:
            raise ValueError("invalid/duplicate documented ID")
        identities.add(cid)
        if row["source_id"] not in source_ids:
            raise ValueError("unknown documented source")
        if row["source_id"] not in {
            "ATTACK_T1485", "ATTACK_T1486", "ATTACK_T1070_004", "ATTACK_T1098"
        }:
            raise ValueError("documented example must reference reviewed ATT&CK procedure")
        for key in ("procedure_id", "actor_or_campaign", "behavior_summary_zh", "evidence_limitation_zh"):
            text_field(row, key)
        if not re.fullmatch(r"(C|S|G)\\d{4}", row["procedure_id"]):
            raise ValueError("invalid public procedure ID")
        tokens(row, "potential_detection_signals")
        tokens(row, "tags", allowed=TAGS)
        if any((row["label"] != "source_reported_malicious_technique",
                row["source_relation"] != "ATTACK_procedure_example",
                row["provenance"] != "public_reference_report",
                row["raw_trace_included"] is not False,
                row["syscall_sequence_verified"] is not False,
                row["agent_attribution_verified"] is not False,
                row["training_ground_truth_usable"] is not False)):
            raise ValueError("source report falsely claimed as executable proof")

    if len(synthetic) < 24 or min(counts.values(), default=0) < 8 or len(counts) != 3:
        raise ValueError("missing risk/benign/unknown balance")
    if len(reported) < 6:
        raise ValueError("insufficient public procedure coverage")
    return {
        "schema": "behavior_analyst_annotation.v1",
        "collected_source_references": len(sources),
        "analyst_constructed_cases": len(synthetic),
        "source_reported_procedures": len(reported),
        "verdict_counts": dict(sorted(counts.items())),
        "real_system_call_traces": 0,
        "independent_human_ground_truth": 0,
        "production_training_authorized": False,
        "shadow_only": True,
    }


if __name__ == "__main__":
    print(json.dumps(audit(), ensure_ascii=False, sort_keys=True, indent=2))

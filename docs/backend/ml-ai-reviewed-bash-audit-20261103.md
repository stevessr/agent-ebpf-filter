# 2026-11-03 — AI command-by-command annotation audit

## Scope and provenance

These are **ChatGPT GPT-6 per-command static judgments**, not independently verified human labels, not measured execution traces, and not proof of system-level detection accuracy.

- Source: https://github.com/rivoluzione-informatica/bash-datasets
- Revision: fcac513317e1487c75f5d3fd0f50f3ddb2ba325a
- Files: data/taglio4_3k/taglio4_safe.jsonl and taglio4_unsafe.jsonl
- Source Git blob SHA-1: 7a4a1553543fd2fc6eb8adc0054365a6eb142b86 / 59d5ce3e5705df4939d57e3cb4807be62da89a32
- Original and adapted-data license: **CC BY-SA 4.0**. Preserve attribution/share-alike on redistribution.

## Verdicts

| Action | Rows | Training eligibility |
| --- | ---: | --- |
| ALLOW | 47 | Candidate-only |
| BLOCK | 52 | Candidate-only |
| DEFER | 25 | Excluded: environment/policy review needed |
| **Total** | **124** | 99 candidate rows |

Three intentional source-label corrections:

- safe → BLOCK: safe file line 614, find piped into xargs with rm -rdf actually performs recursive deletion.
- safe → BLOCK: safe file line 1595, shred may irreversibly overwrite data despite confusing positional arguments.
- unsafe → ALLOW: unsafe file line 238, chmod 0600 id_rsa tightens permissions on an authorized private key.

Files: research/curated/ai-reviewed-bash-actions.jsonl (approved candidate judgments), ai-review-needs-context.jsonl (DEFER) and ai-review-manifest.json (provenance and disagreements). Every record links its original file/line/revision and includes an independent Chinese explanation. No commands were executed.

## Review policy

Assumption: a coding Agent may perform ordinary read-only or bounded workspace actions but has no blanket right to access system credentials, modify /etc/passwd, /etc/sudoers or /etc/shadow, overwrite raw devices, add SUID shells, exfiltrate data, or hijack shell commands.

ALLOW: clearly read-only, or bounded workspace operations. BLOCK: clear unauthorized sensitive data reads, exfiltration, destructive system writes, privilege modifications, persistent shell-command hijacks. DEFER: shell startup variables with uncertain execution, unknown scripts, strace/tcpdump, broad chmod, lvextend, ln --force, path-dependent moves and other context-dependent side effects.

Crucially: scenario_id denotes a **static source curation reason**, and run_id denotes a **source row**, not a real host/session/run. Splitting these cohorts is **not** independent cross-run or cross-host evaluation.

## Running candidate-only training

The corpus curator rejects assistant_reviewed labels by default. It requires an explicit --allow-ai-reviewed to accept approved_for_candidate_only rows, and will not accept needs_context rows. Since the source-corpus categories strongly correlate with labels, this 99-row subset is not a reliable cross-scenario test. Use local candidate training solely as a pipeline smoke test, not a deployment gate.

From repository root:

```sh
python3 -m unittest discover -s scripts -p 'test_ml_curate_corpus.py' -v
python3 -m unittest discover -s scripts -p 'test_ml_reviewed_annotations.py' -v
cd backend
digest="$(sha256sum ../research/curated/ai-reviewed-bash-actions.jsonl | cut -d' ' -f1)"
go run ./cmd/ml-train \
  --dataset-file ../research/curated/ai-reviewed-bash-actions.jsonl \
  --dataset-sha256 "$digest" \
  --dataset-license CC-BY-SA-4.0 \
  --output artifacts/ai-reviewed-model-candidate.bin
```

CI trains, serializes and reloads a short-lived candidate artifact; no policy change or automatic production promotion. Validation accuracy from this small biased set must not be compared to accuracy from the full 2,187-record public training corpus as proof of a gain.

## Remaining work

Review a separate sample of CLI Command Risk v1.5 with pinned row IDs/family splits, then real Agent/eBPF sessions with independently audited policy actions. Evaluate BLOCK→ALLOW misses, precision, benign FPR and latency on source/run/time-disjoint holdouts before promotion.

### Sanitization refinement (2026-11-03 follow-up)

`CleanSensitive` now preserves publicly documented **path names** under `/etc`, notably `/etc/passwd`, `/etc/shadow`, `/etc/sudoers`, `/etc/hosts`, `/etc/group`, `/etc/fstab`, `/etc/resolv.conf` and `/etc/ssh/sshd_config`. These are risk-bearing *targets*, not private file contents. Even tricky cases such as `grep passwd /etc/passwd`, relative traversal to `../../etc/shadow`, and path suffixes no longer lose their semantics through blanket path redaction.

The sanitized stream **still masks** literal credential values (including quoted `--token` and `Authorization: Bearer/Basic` credentials), crypt hash literals, identifying user-home paths, user email addresses, and deployment-specific IP addresses. Widely standardized non-identifying IPv4 (loopback, unspecified, broadcast and RFC 5737 documentation blocks) is now retained.

Regressions in `backend/app/dataset_sanitize_test.go` and `TestPullRemoteDatasetFromClassicCSVWithCleaning` are checked by `ML curated corpus quality` CI. This modifies the feature representation of **newly built candidate models**; existing deployed model artifacts are not retrained or promoted. Candidate metrics must be compared on the same held-out samples and false-positive budgets, not assumed to improve automatically.

**Privacy boundary:** retaining a string naming `/etc/shadow` does not mean returning its contents, making its underlying filesystem access permissible, or preserving embedded crypt password hashes. Authorization remains a separate policy check.

### Stable entity pseudonyms (2026-11-03)

For **new cleaned imports**, identity-aware cleaning retains the shape of
project and user references while removing private names. The mapping is
**local to one dataset import**, and is not uploaded or written to model
manifests. Examples:

| Input (synthetic names) | Cleaned |
| --- | --- |
| `/home/alice/projects/secret-app/src` | `/home/__USER_0001__/projects/__PROJECT_0001__/src` |
| `/home/bob/projects/another-app` | `/home/__USER_0002__/projects/__PROJECT_0002__` |
| `USER=alice PROJECT_NAME=secret-app` | `USER=__USER_0001__ PROJECT_NAME=__PROJECT_0001__` |
| `git@github.com:alice/secret-app.git` | `git@github.com:__USER_0001__/__PROJECT_0001__.git` |

The same recognized name receives the same typed token in the same import
(including independent rows and `CommandLine`/`Args`); distinct names get
distinct tokens. Importing a second, unrelated corpus starts a new local
mapping. Standard `/etc` paths and service accounts such as `root` stay
literal; credentials and private hosts/IPs are **never** treated as usernames
or project names. Paths with no reliable contextual evidence such as
`/tmp/file` are not arbitrarily guessed to be project names.

The mappings are **pseudonymization, not guaranteed anonymity**: context can
still reveal someone indirectly. Dataset labels/provenance and stored model
features remain subject to normal privacy and access controls. Stable tokens
are intentionally *not* derived from unhashed global names or an unsalted
global hash, which would enable straightforward linkage across datasets.

The importer, sanitizer and feature extractor now share identity semantics.
Tests cover Linux/Windows home paths, Git remotes, argv flags, cross-row
consistency, repeated sanitization and secret masking. This is a **candidate
pipeline feature**; no deployed model or runtime policy is changed.


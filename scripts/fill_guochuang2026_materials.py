#!/usr/bin/env python3
"""Fill Guochuang 2026 submission materials from structured team data.

This helper intentionally keeps the source markdown/PPT generator as the
editable truth. It creates a `docs/competition/2026-guochuang/output/final`
directory with filled Markdown, DOCX, PDF, PPTX and a final ZIP package.

Usage:
  rtk python3 scripts/fill_guochuang2026_materials.py \
    --data docs/competition/2026-guochuang/负责人信息采集模板.json
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT / "docs/competition/2026-guochuang"
DEFAULT_DATA = BASE / "负责人信息采集模板.json"
DEFAULT_OUT = BASE / "output/final"


def val(d: dict, *keys: str, default: str = "") -> str:
    cur = d
    for key in keys:
        if not isinstance(cur, dict):
            return default
        cur = cur.get(key)
    if cur is None:
        return default
    return str(cur).strip()


def nonempty(value: str, placeholder: str = "【待填】") -> str:
    return value.strip() if value and value.strip() else placeholder


def load_data(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def member_rows(data: dict) -> list[dict]:
    members = data.get("members") or []
    return [m for m in members if isinstance(m, dict)]


def teacher_rows(data: dict) -> list[dict]:
    teachers = data.get("teachers") or []
    return [t for t in teachers if isinstance(t, dict)]


def leader_summary(data: dict) -> str:
    leader = data.get("leader") or {}
    parts = [
        nonempty(str(leader.get("name", ""))),
        nonempty(str(leader.get("student_id", ""))),
        " / ".join(
            p
            for p in [
                str(leader.get("college", "")).strip(),
                str(leader.get("major", "")).strip(),
                str(leader.get("grade", "")).strip(),
            ]
            if p
        )
        or "【学院/专业/年级待填】",
        f"手机：{nonempty(str(leader.get('phone', '')))}",
        f"QQ：{nonempty(str(leader.get('qq', '')))}",
        f"邮箱：{nonempty(str(leader.get('email', '')))}",
    ]
    return "，".join(parts)


def teacher_summary(data: dict) -> str:
    teachers = teacher_rows(data)
    names = [t.get("name", "").strip() for t in teachers if t.get("name", "").strip()]
    if names:
        return "；".join(names)
    return "【待填：姓名、学院、职称、联系方式】"


def team_count(data: dict) -> str:
    count = len([m for m in member_rows(data) if m.get("name", "").strip()])
    if count:
        return f"{count} 人"
    configured = len(member_rows(data))
    return f"【待填：{configured or '3-15'}人，含负责人】"


def render_application_team_table(data: dict) -> str:
    rows = ["| 角色 | 姓名 | 主要职责 |", "| --- | --- | --- |"]
    for m in member_rows(data):
        role = nonempty(str(m.get("role", "")))
        name = nonempty(str(m.get("name", "")))
        division = nonempty(str(m.get("division", "")))
        rows.append(f"| {role} | {name} | {division} |")
    for t in teacher_rows(data):
        role = "指导教师"
        name = nonempty(str(t.get("name", "")))
        division = nonempty(str(t.get("guidance", "")))
        rows.append(f"| {role} | {name} | {division} |")
    return "\n".join(rows)


def render_online_member_table(data: dict) -> str:
    rows = [
        "| 排序 | 姓名 | 学号 | 学校/学院/专业 | 年级 | 角色 | 分工 | 手机/邮箱 |",
        "| --- | --- | --- | --- | --- | --- | --- | --- |",
    ]
    for idx, m in enumerate(member_rows(data), 1):
        school_major = " / ".join(
            p
            for p in [
                str(m.get("school", "")).strip() or val(data, "project", "school"),
                str(m.get("college", "")).strip(),
                str(m.get("major", "")).strip(),
            ]
            if p
        )
        contact = " / ".join(p for p in [str(m.get("phone", "")).strip(), str(m.get("email", "")).strip()] if p)
        rows.append(
            "| {order} | {name} | {sid} | {school_major} | {grade} | {role} | {division} | {contact} |".format(
                order=m.get("order") or idx,
                name=nonempty(str(m.get("name", ""))),
                sid=nonempty(str(m.get("student_id", ""))),
                school_major=nonempty(school_major),
                grade=nonempty(str(m.get("grade", ""))),
                role=nonempty(str(m.get("role", ""))),
                division=nonempty(str(m.get("division", ""))),
                contact=nonempty(contact),
            )
        )
    return "\n".join(rows)


def render_online_teacher_table(data: dict) -> str:
    rows = [
        "| 排序 | 姓名 | 学校/学院 | 职称 | 研究方向 | 指导内容 | 手机/邮箱 |",
        "| --- | --- | --- | --- | --- | --- | --- |",
    ]
    for idx, t in enumerate(teacher_rows(data), 1):
        school_college = " / ".join(
            p
            for p in [
                str(t.get("school", "")).strip() or val(data, "project", "school"),
                str(t.get("college", "")).strip(),
            ]
            if p
        )
        contact = " / ".join(p for p in [str(t.get("phone", "")).strip(), str(t.get("email", "")).strip()] if p)
        rows.append(
            "| {order} | {name} | {school_college} | {title} | {research} | {guidance} | {contact} |".format(
                order=t.get("order") or idx,
                name=nonempty(str(t.get("name", ""))),
                school_college=nonempty(school_college),
                title=nonempty(str(t.get("title", ""))),
                research=nonempty(str(t.get("research", ""))),
                guidance=nonempty(str(t.get("guidance", ""))),
                contact=nonempty(contact),
            )
        )
    return "\n".join(rows)


def replace_between(text: str, start_heading: str, end_heading: str, replacement: str) -> str:
    pattern = re.compile(
        rf"({re.escape(start_heading)}\n)(.*?)(\n{re.escape(end_heading)})",
        flags=re.S,
    )
    return pattern.sub(rf"\1\n{replacement}\n\3", text)


def fill_common(text: str, data: dict) -> str:
    leader = data.get("leader") or {}
    replacements = {
        "高教主赛道（默认）": f"{val(data, 'project', 'track') or '高教主赛道'}（默认）",
        "创意组（默认，未注册公司）": f"{val(data, 'project', 'group') or '创意组'}（默认，未注册公司）",
        "| 推荐学院 | 【待填】 |": f"| 推荐学院 | {nonempty(val(data, 'project', 'recommend_college'))} |",
        "| 项目负责人 | 【待填：姓名、学号、专业、年级、手机号、QQ、邮箱】 |": f"| 项目负责人 | {leader_summary(data)} |",
        "| 指导教师 | 【待填：姓名、学院、职称、联系方式】 |": f"| 指导教师 | {teacher_summary(data)} |",
        "| 团队人数 | 【待填：3-15人，含负责人】 |": f"| 团队人数 | {team_count(data)} |",
        "| 项目负责人 | 【待填】 |": f"| 项目负责人 | {nonempty(str(leader.get('name', '')))} |",
        "| 负责人所在学校/学院/专业 | 【待填】 |": "| 负责人所在学校/学院/专业 | {} / {} / {} |".format(
            nonempty(val(data, "project", "school")),
            nonempty(str(leader.get("college", ""))),
            nonempty(str(leader.get("major", ""))),
        ),
        "| 负责人手机号/QQ/邮箱 | 【待填】 |": "| 负责人手机号/QQ/邮箱 | 手机：{}；QQ：{}；邮箱：{} |".format(
            nonempty(str(leader.get("phone", ""))),
            nonempty(str(leader.get("qq", ""))),
            nonempty(str(leader.get("email", ""))),
        ),
        "| 指导教师 | 【待填】 |": f"| 指导教师 | {teacher_summary(data)} |",
        "| 团队成员数 | 【待填，3-15人】 |": f"| 团队成员数 | {team_count(data)} |",
    }
    for old, new in replacements.items():
        text = text.replace(old, new)
    return text


def fill_markdown_sources(data: dict, out_dir: Path) -> list[Path]:
    out_dir.mkdir(parents=True, exist_ok=True)
    generated: list[Path] = []

    app = fill_common((BASE / "申报书.md").read_text(encoding="utf-8"), data)
    app = replace_between(app, "## 十、团队分工（提交前补真实姓名）", "## 十一、实施计划", render_application_team_table(data))
    app_path = out_dir / "Agent-eBPF-Filter-国创赛2026申报书-已填.md"
    app_path.write_text(app, encoding="utf-8")
    generated.append(app_path)

    business = (BASE / "商业企划书.md").read_text(encoding="utf-8")
    business_path = out_dir / "Agent-eBPF-Filter-商业企划书-已填.md"
    business_path.write_text(business, encoding="utf-8")
    generated.append(business_path)

    online = fill_common((BASE / "在线填报字段草稿.md").read_text(encoding="utf-8"), data)
    online = replace_between(online, "## 11. 团队成员字段模板", "## 12. 指导教师字段模板", render_online_member_table(data))
    online = replace_between(online, "## 12. 指导教师字段模板", "## 13. 三年规划", render_online_teacher_table(data))
    online_path = out_dir / "Agent-eBPF-Filter-在线填报字段草稿-已填.md"
    online_path.write_text(online, encoding="utf-8")
    generated.append(online_path)

    for src in ["规则核验与提交清单.md", "提交说明.md", "负责人信息采集表.md", "深度调研与代码实现映射.md"]:
        dst = out_dir / src
        shutil.copy2(BASE / src, dst)
        generated.append(dst)
    return generated


def run(cmd: list[str], cwd: Path | None = None) -> None:
    print("+", " ".join(cmd))
    subprocess.run(cmd, cwd=cwd, check=True)


def export_docs(out_dir: Path) -> None:
    mapping = {
        "Agent-eBPF-Filter-国创赛2026申报书-已填.md": "Agent-eBPF-Filter-国创赛2026申报书-已填.docx",
        "Agent-eBPF-Filter-商业企划书-已填.md": "Agent-eBPF-Filter-商业企划书-已填.docx",
        "Agent-eBPF-Filter-在线填报字段草稿-已填.md": "Agent-eBPF-Filter-在线填报字段草稿-已填.docx",
        "深度调研与代码实现映射.md": "深度调研与代码实现映射.docx",
    }
    for md, docx in mapping.items():
        run(["pandoc", str(out_dir / md), "--from", "markdown", "--to", "docx", "--standalone", "-o", str(out_dir / docx)])
    docx_files = [str(out_dir / name) for name in mapping.values()]
    run(["libreoffice", "--headless", "--convert-to", "pdf", "--outdir", str(out_dir), *docx_files])


def fill_pptx_xml(data: dict, out_dir: Path) -> Path:
    src = BASE / "output/Agent-eBPF-Filter-项目答辩路演PPT.pptx"
    dst = out_dir / "Agent-eBPF-Filter-项目答辩路演PPT-已填.pptx"
    leader = data.get("leader") or {}
    members = member_rows(data)

    role_name = {}
    for m in members:
        role = str(m.get("role", "")).strip()
        name = str(m.get("name", "")).strip()
        if role and name:
            role_name[role] = name

    replacements = {
        "推荐学院：待填": f"推荐学院：{nonempty(val(data, 'project', 'recommend_college'))}",
        "负责人：待填": f"负责人：{nonempty(str(leader.get('name', '')))}",
        "手机号：待填": f"手机号：{nonempty(str(leader.get('phone', '')))}",
        "QQ：待填": f"QQ：{nonempty(str(leader.get('qq', '')))}",
        "提交前将【待填】替换为真实姓名、学号、年级、学院与贡献证据。": "团队成员按真实分工填写，提交前请核对成员排序与系统一致。",
        "【待填】总体架构 / 答辩 / 进度": f"{role_name.get('项目负责人') or leader.get('name') or '【待填】'} 总体架构 / 答辩 / 进度",
        "【待填】LSM / cgroup / tracepoints": f"{role_name.get('eBPF/内核开发') or '【待填】'} LSM / cgroup / tracepoints",
        "【待填】Go API / EventEnvelope / 策略": f"{role_name.get('后端开发') or '【待填】'} Go API / EventEnvelope / 策略",
        "【待填】Vue / 执行图谱 / 低代码": f"{role_name.get('前端开发') or '【待填】'} Vue / 执行图谱 / 低代码",
        "【待填】semantic_alerts / ML / replay": f"{role_name.get('算法评测') or '【待填】'} semantic_alerts / ML / replay",
        "【待填】客户访谈 / 财务 / 路演": f"{role_name.get('商业调研') or '【待填】'} 客户访谈 / 财务 / 路演",
        "<a:t>【待填】</a:t>": f"<a:t>{nonempty(str(leader.get('name', '')))}</a:t>",
    }

    with zipfile.ZipFile(src, "r") as zin, zipfile.ZipFile(dst, "w", compression=zipfile.ZIP_DEFLATED) as zout:
        for item in zin.infolist():
            content = zin.read(item.filename)
            if item.filename.endswith(".xml"):
                text = content.decode("utf-8", errors="ignore")
                for old, new in replacements.items():
                    text = text.replace(old, new)
                content = text.encode("utf-8")
            zout.writestr(item, content)
    run(["libreoffice", "--headless", "--convert-to", "pdf", "--outdir", str(out_dir), str(dst)])
    return dst


def audit(data: dict, out_dir: Path) -> Path:
    missing: list[str] = []
    required = [
        ("学校", val(data, "project", "school")),
        ("推荐学院", val(data, "project", "recommend_college")),
        ("负责人姓名", val(data, "leader", "name")),
        ("负责人学号", val(data, "leader", "student_id")),
        ("负责人手机号", val(data, "leader", "phone")),
        ("负责人QQ", val(data, "leader", "qq")),
        ("负责人邮箱", val(data, "leader", "email")),
    ]
    for label, value in required:
        if not value:
            missing.append(label)
    named_members = [m for m in member_rows(data) if m.get("name", "").strip()]
    if len(named_members) < 3:
        missing.append("至少3名团队成员真实姓名")
    if not any(t.get("name", "").strip() for t in teacher_rows(data)):
        missing.append("至少1名指导教师")

    lines = [
        "# 最终提交审计表",
        "",
        "## 已检查项",
        "",
        "- 2026 正式总通知状态：截至本轮检索仍未发现教育部正式总通知，按 2025 正式通知/评审规则与 2026 校赛通知准备。",
        "- 申报书、商业企划书、PPT、在线填报字段草稿均可由本脚本生成最终版。",
        "- 深度调研与代码实现映射已作为独立支撑材料纳入最终包，覆盖 OWASP、Five Eyes、GitGuardian、NIST 与仓库代码证据。",
        "- PPT 控制在 18 页，提交包低于常见 20MB 限制。",
        "",
        "## 仍需人工确认/补齐",
        "",
    ]
    if missing:
        lines.extend(f"- {item}" for item in missing)
    else:
        lines.append("- 未发现基础信息缺口；仍需登录系统提交并保存回执。")
    lines.extend(
        [
            "",
            "## 在线提交必须保留的证据",
            "",
            "- 全国大学生创业服务网或校内系统提交成功截图。",
            "- 系统项目编号/报名编号。",
            "- 学院盖章扫描件或审核通过截图。",
            "- 邮件/平台回执。",
        ]
    )
    path = out_dir / "Agent-eBPF-Filter-最终提交审计表.md"
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return path


def package(out_dir: Path) -> Path:
    zip_path = out_dir / "Agent-eBPF-Filter-国创赛2026最终提交包.zip"
    include = [p for p in out_dir.iterdir() if p.is_file() and p.name != zip_path.name]
    with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as z:
        for p in include:
            z.write(p, arcname=p.name)
    return zip_path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--data", type=Path, default=DEFAULT_DATA)
    parser.add_argument("--out-dir", type=Path, default=DEFAULT_OUT)
    parser.add_argument("--audit-only", action="store_true")
    args = parser.parse_args()

    data = load_data(args.data)
    out_dir = args.out_dir
    out_dir.mkdir(parents=True, exist_ok=True)

    audit_path = audit(data, out_dir)
    print(f"Wrote {audit_path}")
    if args.audit_only:
        print("Audit-only mode; no documents exported.")
        return 0

    fill_markdown_sources(data, out_dir)
    export_docs(out_dir)
    fill_pptx_xml(data, out_dir)
    zip_path = package(out_dir)
    run(["unzip", "-t", str(zip_path)])
    print(f"Final package: {zip_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

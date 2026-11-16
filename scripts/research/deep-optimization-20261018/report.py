"""Publish aggregate experiment evidence; raw samples and weights stay separate."""
import argparse,json,shutil
from pathlib import Path

def pct(v):return '—' if v is None else f'{100*v:.2f}%'
def fraction(r):return f'{r["fp"]}/{r["tn"]+r["fp"]} ({pct(r["fpr"])})'
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir(exist_ok=True);load=lambda p:json.loads((a.root/p).read_text());families=['privacy','http','http_structured','http_v2','http_structured_v3'];results={n:load(f'{n}/results.json') for n in families};audit=load('evaluation/audit.json');live=load('live/results.json');pi=load('evaluation/pisas_results.json');crs=load('evaluation/crs_results.json');crs2=load('evaluation/crs_v2_results.json');baseline=load('evaluation/threshold_only_baseline.json');p=results['privacy'];q=p['consumed_test'];bm=baseline['consumed_test']
    if not audit['all_checks_pass']:raise ValueError('cannot publish failed evidence audit')
    for family in families:
        for name in ['protocol','freeze','results','trials','features','data_audit']:
            src=a.root/family/(name+'.json')
            if src.exists():shutil.copyfile(src,a.out/(family+'_'+name+'.json'))
    for name in ['audit','pisas_results','pisas_source_hashes','threshold_only_baseline','nuisance_results','matched_benign_comparison','crs_results','crs_v2_results']:
        shutil.copyfile(a.root/'evaluation'/(name+'.json'),a.out/(name+'.json'))
    for name in ['results','runs','protocol']:shutil.copyfile(a.root/'live'/(name+'.json'),a.out/('live_'+name+'.json'))
    title={'http':'词法 v1','http_structured':'Go 特征 v2','http_v2':'词法修正版 v2','http_structured_v3':'Go 比率/选择器修正版 v3'}
    lines=['# 公开数据深度训练与实际权重测试（2026-10-18）','',
      '**完成 43 次实际拟合：40 个风险分类器配置、3 个特征选择器；两个 BERT 各训练 6 epoch。生产门槛仍未通过。** 所有结果使用真实权重；本轮没有启用生产拦截。','',
      '## 隐私模型','',
      f'验证集选择了 **BERT-Mini 第 {p["freeze"]["selection"]["selected_epoch"]} 个 epoch**。与旧 BERT 使用同一份独立校准分区、同一经验 1% 误报操作点比较：','',
      '| 模型 / 382 条已消费回归样本 | 准确率 | 泄漏召回 | 误报率 | AP | AUROC |',
      '|---|---:|---:|---:|---:|---:|',
      f'| 旧模型，仅校准阈值 | {pct(bm["accuracy"])} | {pct(bm["recall"])} | {pct(bm["fpr"])} | {bm["AP"]:.4f} | {bm["AUROC"]:.4f} |',
      f'| 新训练 BERT-Mini | {pct(q["accuracy"])} | {pct(q["recall"])} | {pct(q["fpr"])} | {q["AP"]:.4f} | {q["AUROC"]:.4f} |','',
      f'新模型 TP={q["tp"]}、FP={q["fp"]}、TN={q["tn"]}、FN={q["fn"]}，阈值={p["freeze"]["threshold"]:.12f}。排序指标提高，低误报处的召回**没有超过阈值对照**。99 条校准负例不足以证明真实 FPR≤1%；151 条回归负例零误报的一侧 95% 上界仍为 {pct(q["fpr_upper95"])}。','',
      f'新来源 PiSAs：{pi["scenario_count"]} 个场景、{pi["overall"]["n"]} 个属性动作适配样本，召回 {pct(pi["overall"]["recall"])}、误报 {pct(pi["overall"]["fpr"])}、AP={pi["overall"]["AP"]:.4f}。这是任务适当性属性披露迁移诊断，**不是原始 PiSAs Agent 执行分数或真实隐私泄漏率**。输入排除了标注枚举、可见性列表、private archetype 和 oracle。','',
      '## HTTP 固定回归与新规则留出','',
      '所有候选按验证集选择，校准集冻结阈值。ECML/CSIC 已消费，校正后复用的正常审计也属于回归诊断。新规则留出包含不同 CRS rule ID，移除先前挑战中重复的请求；它与第一次挑战共享 CRS 来源。','',
      '| 模型 | ECML 召回 / FPR | CSIC 召回 / FPR | 同一 9,288 条正常审计 FPR | 新规则挑战命中 / 样本 |',
      '|---|---:|---:|---:|---:|']
    for family in families[1:]:
        r=results[family]['consumed_regression'];e=r['public_ecml_generated'];c=r['public_csic_generated'];b=audit['matched_benign_comparison'][family];x=crs2['models'][family]
        lines.append(f'| {title[family]} | {pct(e["recall"])} / {pct(e["fpr"])} | {pct(c["recall"])} / {pct(c["fpr"])} | {fraction(b)} | {x["hits"]}/{x["n"]} ({pct(x["recall"])}) |')
    lines+=['',f'首次独立来源 CRS 挑战 {crs["n"]} 条：词法 v1 命中 {crs["models"]["http"]["hits"]}/{crs["n"]}，Go 特征 v2 命中 {crs["models"]["http_structured"]["hits"]}/{crs["n"]}。CRS 样本的标签只代表“预期触发指定规则”；没有执行 payload、CRS 引擎或利用链，不能称为成功攻击拦截率。', '',
      'Go 修正版的高攻击命中与高正常误报同时保留。它没有通过生产质量要求，也没有替代词法候选。语义修复和训练收益分别评估。','',
      f'正常审计的重复/模板诊断：词法 v1 在 {audit["fresh_group_diagnostics"]["unique_payload"]["n"]} 个唯一 payload 上 FPR={pct(audit["fresh_group_diagnostics"]["unique_payload"]["fpr"])}；在 {audit["fresh_group_diagnostics"]["unseen_template_rows"]["n"]} 条模板也未见过的请求上 FPR={pct(audit["fresh_group_diagnostics"]["unseen_template_rows"]["fpr"])}。共享生成器和模板相关性使行级二项分布区间不能作为生产保证。','',
      '自动工程过程：137 个真实 Go 特征加缺失标记，派生对数计数、按字段长度归一化的比率和提示交互；训练分区内部去常量、重要性排序、相关性去重，最多选 32 列。后续版本修复 body 分母、保留 HTTP 原始 `#` 后缀，并比较按来源条件计算重要性的选择器。未观测 syscall/path/history 字段始终缺失；不存在用 HTTP 标签训练出的破坏性 syscall 结论。','',
      '## 新 OpenCode 会话：实际执行前模型推理','',
      '| 会话 | Native 调用 | 实际模型评分 | 任务完成 | 超时 | 报告费用 |',
      '|---|---:|---:|---|---|---:|']
    for r in live['runs']:lines.append(f'| {r["case"]} | {r["native_calls"]} | {r["scored_calls"]} | {r["task_complete"]} | {r["timeout"]} | {r["reported_cost"]} |')
    paired=audit['matched_privacy_comparison'];np=live['privacy']
    lines+=['',f'同一批 {np["n"]} 条真实正常候选：旧阈值模型回放 FPR={pct(paired["old_legacy_threshold"]["fpr"])}，旧模型仅校准阈值回放 FPR={pct(paired["old_calibration_only"]["fpr"])}，新模型执行前实时推理 FPR={pct(np["fpr"])}。没有泄漏正例，不能从本表估计拦截召回。','',
      '| 实际收到的正常 HTTP：每个模型同样 16 条 | 普通 API / 文档 12 条误报 | 合法代码示例上传 4 条误报 |',
      '|---|---:|---:|']
    for family in families[1:]:
        r=live['http'][family];lines.append(f'| {title[family]} | {fraction(r["ordinary"])} | {fraction(r["code_docs"])} |')
    lines+=['',
      '上述请求只发往自有回环 fixture；SQL、HTML、shell 示例作为文档数据存储，没有执行。一个请求被四个模型重复评分，分母仍是 16 个实际请求。所有会话使用固定免费模型 `opencode/deepseek-v4.1-flash-free`；费用是 native 日志报告值。','',
      f'完整性审计：全部模型/协议摘要、数据划分摘要、native call ID、输入摘要和日志摘要核对通过；评分重载最大差异 {audit["privacy_reload_max_score_difference"]:.3g}。实际 BLOCK=0，全部为影子阈值判断。四个输入语义测试及公开数据重新准备的逐字节哈希一致性通过。','',
      '## 生产资格和交付','',
      '**继续保持 draft / shadow-only。** 主要未通过项是隐私低误报召回、跨模板正常 HTTP 误报，以及独立真实 syscall 风险标签。新增 Python 模型也尚未满足现有 Go worker 的编码/资源/部署契约，不能直接替换其生产服务。需要增加公开且可核验的长上下文动作隐私/授权样本、独立来源正常 HTTP、具备实际结果的 syscall 数据，再重新冻结训练/校准/测试协议。','',
      '| 冻结模型 | SHA-256 |', '|---|---|']
    for family in families:lines.append(f'| {family} | `{audit["artifacts"][family]}` |')
    lines+=['','完整原始分数、native 日志、各冻结候选的实际权重、校准协议和数据摘要另附证据包。此目录发布可复现代码与聚合结果，不在 Git 中重复公开原始语料。未持久化的未选中树/修正版词法权重可按固定协议重训；所有候选的验证指标均保留在 `*_trials.json`。参见 [复现说明](README.md)，失败结果没有改写为成功。','']
    (a.out/'RESULTS.md').write_text('\n'.join(lines));print('Aggregate report written:',a.out/'RESULTS.md')
if __name__=='__main__':main()

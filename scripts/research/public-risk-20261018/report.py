from pathlib import Path
import json,numpy as np,hashlib
from autosearch import ROOT

def main():
 ar=ROOT/'artifacts';results={(t,b):json.loads((ar/f'{t}_{b}_results.json').read_text()) for t in ['privacy','code'] for b in ['tiny','new']};manifest=json.loads((ROOT/'data/manifest.json').read_text());cert=json.loads((ar/'certificate.json').read_text())
 text='# 第二轮：更换底座与自动优化实测（2026-10-18）\n\n'
 text+='本轮已换用 **MiniLM-L6-v2（隐私）与 CodeT5-small encoder（代码）**，冻结编码器后实际训练线性／深度分类头，运行有限预算自动配置搜索。没有将冻结编码器实验描述为全参数底座微调。\n\n'
 text+='研究方法参考 AutoProver/AIProver 的候选验证与诊断反馈、autoresearch 的固定预算 keep/discard 流程，以及先定义验收规则的 AutoSciRub。实现的是可执行实验合同与有限搜索；没有运行 Solidity/Lean 证明器，也不构成模型正确性的形式化证明。来源与固定模型版本见 README.md 和 program.md。\n\n'
 code_new=results[('code','new')]['confirmation_seed_mean'];code_old=results[('code','tiny')]['confirmation_seed_mean'];privacy_new=results[('privacy','new')]['confirmation_seed_mean'];privacy_old=results[('privacy','tiny')]['confirmation_seed_mean']
 text+='## 本轮结论\n\n'
 text+=f'- **代码方案保留为有改善的研究候选**：确认集 balanced accuracy {code_old["balanced_accuracy"]:.1%} → {code_new["balanced_accuracy"]:.1%}，AP {code_old["AP"]:.3f} → {code_new["AP"]:.3f}。仍未通过部署验收，不能用于自动阻断。\n'
 text+=f'- **隐私迁移没有证实收益，不作为升级发布**：MiniLM 方案确认集 balanced accuracy {privacy_new["balanced_accuracy"]:.1%}，Tiny 对照 {privacy_old["balanced_accuracy"]:.1%}。验证集改善没有复现，不能以验证分数替代确认结果。\n'
 text+='- 32 组配置、全部 keep/discard、四组胜出配置的三种子结果均保留。发布的是本轮已冻结模型和诊断材料；没有使用确认结果继续调参。\n\n'
 text+='## 数据隔离\n\n旧测试集已在第一轮查看，本轮只称“回归集”。确认集在新底座／分类头训练前从旧训练 groups 中留出，所有对照都排除这些 groups。这是同一语料内的新一轮确认，不是新采集的外部测试。\n\n| 任务 | Train | Validation | Calibration | Confirmation | Regression |\n|---|---:|---:|---:|---:|---:|\n'
 for task in ['privacy','code']:text+='| '+task+' | '+' | '.join(str(manifest[task][s]['n']) for s in ['train','validation','calibration','confirmation','regression'])+' |\n'
 text+='\nPrivacyAlign 为公开人工标注的合成场景；代码数据仍为 Devign 缺陷代理标签。实际破坏性动作结果、真实授权与 eBPF 行为闭环仍未由这些数据验证。\n\n'
 def table(split,title):
  s='## '+title+'\n\n| 任务／底座 | Accuracy | Balanced acc. | Recall | FPR | AP |\n|---|---:|---:|---:|---:|---:|\n'
  for (task,base),r in results.items():
   mm=[v['splits'][split]['metrics'] for v in r['replications']];vals=[]
   for k in ['accuracy','balanced_accuracy','recall','FPR','AP']:
    v=np.array([m[k] for m in mm]);vals.append(f'{v.mean():.3f} ± {v.std(ddof=1):.3f}')
   s+='| '+task+' / '+('Tiny BERT' if base=='tiny' else 'MiniLM' if task=='privacy' else 'CodeT5')+' | '+' | '.join(vals)+' |\n'
  return s+'\n均值 ± 样本标准差来自三个分类头种子。底座固定，因此不代表编码器训练的不确定性。阈值只从 calibration 集选择，约束其经验 FPR≤10%，未用确认／回归标签调阈值。\n\n'
 text+=table('confirmation','本轮确认集：主要结果')
 text+='## 配对差异与验收\n\n'
 paired={};decisions={}
 for task in ['privacy','code']:
  pred={b:[json.loads(x) for x in (ar/f'{task}_{b}_predictions.jsonl').read_text().splitlines() if json.loads(x)['split']=='confirmation' and json.loads(x)['seed']==20261018] for b in ['tiny','new']}
  a={r['id']:r for r in pred['tiny']};b={r['id']:r for r in pred['new']};ids=sorted(a);assert set(a)==set(b)
  # Group IDs come from the sealed cache, not model features.
  data=dict(np.load(ar/f'{task}_tiny/confirmation.npz',allow_pickle=False));gmap=dict(zip(data['ids'].tolist(),data['groups'].tolist()));gs=sorted(set(gmap.values()));ix={g:[i for i in ids if gmap[i]==g] for g in gs};rng=np.random.default_rng(20261018);deltas=[]
  from sklearn.metrics import balanced_accuracy_score
  for _ in range(500):
   sample=[i for g in rng.choice(gs,len(gs),replace=True) for i in ix[g]];y=[a[i]['label'] for i in sample];deltas.append(balanced_accuracy_score(y,[b[i]['prediction'] for i in sample])-balanced_accuracy_score(y,[a[i]['prediction'] for i in sample]))
  ci=np.quantile(deltas,[.025,.975]).tolist();delta=results[(task,'new')]['confirmation_seed_mean']['balanced_accuracy']-results[(task,'tiny')]['confirmation_seed_mean']['balanced_accuracy'];paired[task]={'mean_three_seed_delta_BA':delta,'seed0_paired_group_bootstrap_95CI':ci}
  passes=[x['splits']['confirmation']['deployment_gate_passed'] for x in results[(task,'new')]['replications']]
  lowpasses=[x['splits']['confirmation']['low_FPR_gate_passed'] for x in results[(task,'new')]['replications']]
  decisions[task]={'deployment_gate_passed_all_head_seeds':all(passes),'low_FPR_gate_passed_all_head_seeds':all(lowpasses),'paired_comparison_CI':ci,'paired_comparison_direction':'positive' if ci[0]>0 else 'not_established','automatic_enforcement':False,'note':'descriptive frozen comparison; no further search based on confirmation'}
  text+=f'- **{task}**：新底座相对同协议 Tiny 对照，三种子平均 balanced accuracy 差值 {delta:+.3f}；主种子配对 group bootstrap 95% CI [{ci[0]:+.3f}, {ci[1]:+.3f}]。确认集 Recall≥70% 且 FPR≤5% 的三个种子通过情况：`{passes}`。\n'
 text+='\n验收门槛只是一项实验条件。通过也不自动等于生产安全；未通过明确保留失败，不改动内核策略。差异包含所选分类头及其预算效果，不把它解释成纯底座能力的因果估计。\n\n'
 text+=table('regression','旧测试回归：探索性对照')
 text+='## 自动实验账本与成本\n\n| 任务／底座 | 选择配置 | 搜索 trial 数 | keep / discard / error | 编码成本(s) |\n|---|---|---:|---|---:|\n'
 for (t,b),r in results.items():
  f=r['frozen'];tr=[x for x in f['trials'] if x['status']!='seed_replication'];counts={s:sum(x['status']==s for x in tr) for s in ['keep','discard','error']};ex=f['feature_extraction'];text+=f'| {t}/{b} | `{json.dumps(f["selected_config"])}` | {len(tr)} | {counts["keep"]} / {counts["discard"]} / {counts["error"]} | {ex["wall_seconds"]:.1f} |\n'
 text+='\n每组 fit/selection 预算 30 秒；预训练编码的缓存成本单列，不宣称不同底座总计算成本相同。每个任务的 16 组配置完整保留，之后只复跑选定配置的两个额外种子。源码从配置选择前固定，评测后的搜索会被脚本拒绝。\n\n'
 text+='## 低误报运行点\n\n| 任务／新底座 | 确认 Recall | 确认 FPR | 回归 Recall | 回归 FPR |\n|---|---:|---:|---:|---:|\n'
 for task in ['privacy','code']:
  rr=results[(task,'new')]['replications'];vals=[np.mean([v['splits'][s]['low_FPR'][k] for v in rr]) for s in ['confirmation','regression'] for k in ['recall','FPR']];text+='| '+task+' | '+' | '.join(f'{v:.3f}' for v in vals)+' |\n'
 text+='\n此阈值的≤5%约束发生在 calibration 上，不保证未来数据也保持≤5%。\n\n'
 text+='## 检查与局限\n\n'
 text+='数据散列、group 隔离、有限特征、源样本顺序／标签对应、冻结权重散列均通过可执行检查；数字 JSON 导出与本地已训练模型的预测一致性见 `head_exports.json`。确认／回归逐样本预测和全部指标、置信区间保留。\n\n'
 for task in ['privacy','code']:
  m=results[(task,'new')]['frozen']['feature_extraction'];cov=m['coverage'];text+=f'- {task} 新底座编码 {m["encoded_sequences"]} 个片段、{m["encoder_parameters"]:,} 个冻结参数；受预算选择的 token 覆盖 {cov["selected_tokens"]}/{cov["original_tokens"]}，截断单元 {cov["truncated_units"]}/{cov["units"]}。这不是无损完整上下文。\n'
 text+='\n重复验证搜索、语料近重复、标注分歧和预训练重叠仍可能影响结论。这里的证书证明的是有限实验条件通过，不证明标签、真实授权、因果关系或模型语义安全。尚无真实 Agent/eBPF 回放闭环和中文评测，也没有把“代码有缺陷”升级为“Agent 造成漏洞／破坏”。\n'
 (ROOT/'RESULTS.md').write_text(text);(ar/'paired_comparisons.json').write_text(json.dumps(paired,indent=2));(ar/'acceptance.json').write_text(json.dumps(decisions,indent=2));print('report created',len(text))
if __name__=='__main__':main()

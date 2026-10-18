import json
from pathlib import Path
import numpy as np
from paths import ROOT
from train import OUT,SEED
KEYS=['accuracy','balanced_accuracy','F1','AP','recall','FPR']
def load(name):return json.loads((OUT/name).read_text())
def models(task,which):
    rr=load(task+'_results.json')
    names=([task+'_winner']+[task+'_seed'+str(s) for s in [SEED+1,SEED+2]]) if which=='selected' else ([task+'_tiny_best']+[task+'_control_seed'+str(s) for s in [SEED+1,SEED+2]])
    return [next(r for r in rr if r['model']==n) for n in names]
def stats(rr,split):return {k:{'mean':float(np.mean([r['splits'][split][k] for r in rr])),'std':float(np.std([r['splits'][split][k] for r in rr],ddof=1))} for k in KEYS}
def paired(task):
    rr=[models(task,w) for w in ['control','selected']];dd=[[dict(np.load(OUT/f'{r["model"]}_audit_predictions.npz',allow_pickle=False)) for r in group] for group in rr];y=dd[0][0]['y'];groups=dd[0][0]['groups']
    for group in dd:
        for d in group:assert np.array_equal(d['y'],y) and np.array_equal(d['groups'],groups)
    predictions=[np.stack([d['p']>=d['threshold'] for d in group]).mean(0) for group in dd]
    def delta(ii):return .5*(np.mean((predictions[1]-predictions[0])[ii][y[ii]==1])-np.mean((predictions[1]-predictions[0])[ii][y[ii]==0]))
    unique=np.unique(groups);ix={g:np.flatnonzero(groups==g) for g in unique};rng=np.random.default_rng(SEED);sample=[]
    for _ in range(1000):
        ii=np.concatenate([ix[g] for g in rng.choice(unique,len(unique),replace=True)])
        if len(np.unique(y[ii]))==2:sample.append(delta(ii))
    return {'mean_seed_BA_delta':float(delta(np.arange(len(y)))),'paired_group_bootstrap_95CI':np.quantile(sample,[.025,.975]).tolist(),'bootstrap_replicates':len(sample),'scope':'conditional on these 3 trained seeds; within-corpus audit, not external validation'}
def pct(x):return f'{100*x:.2f}%'
def main():
    summary={};pair={};lines=['# Round 3：前端/后端网络搜索与 OpenCode 白样本实验','',
    '完成 34 组预定架构/超参数试验：30 组冻结编码器+判别头，4 组 TinyBERT 全参数训练。另对胜出配置和 TinyBERT 对照各补 2 个随机种子，并训练 3 个加入白样本的对照模型。所有分数来自实际运行。',
    '', '## 数据与评估边界','',
    'audit 为本轮训练前新划出的组隔离留出集，但来自此前使用过的公开训练语料；不是全新外部测试集。旧 confirmation/test 仅作为回归集。验证集选结构，独立 calibration 选阈值（经验 FPR≤5%），然后冻结再评估。所有模型从公开预训练底座或新判别头开始，没有继承之前微调的头。',
    '', '| 任务 | train | validation | calibration | 本轮 audit | 旧 confirmation | 旧 test |','|---|---:|---:|---:|---:|---:|---:|']
    manifest=load('data_manifest.json')
    for task in ['privacy','code']:lines.append('| '+task+' | '+' | '.join(str(manifest[task][s]['n']) for s in ['train','validation','calibration','audit','previous_confirmation','regression'])+' |')
    lines+=['','隐私为 PrivacyAlign 人工泄露标签；代码为 Devign 缺陷代理标签，不能称为真实 Agent 漏洞利用或破坏结果准确率。','', '## 每个前端的验证集最佳结构（单搜索种子）','', '| 任务 | 前端 | 后端 | Val BA | Val AP | Val FPR |','|---|---|---|---:|---:|---:|']
    for task in ['privacy','code']:
        trials=[json.loads(l) for l in (OUT/f'{task}_trials.jsonl').read_text().splitlines()]
        for front in dict.fromkeys(r['config']['front'] for r in trials):
            r=max([r for r in trials if r['config']['front']==front],key=lambda x:x['objective']);m=r['validation'];cfg=r['config'];lines.append(f'| {task} | {front} | {cfg["back"]}'+(f' lr={cfg["lr"]}' if 'lr' in cfg else '')+f' | {pct(m["balanced_accuracy"])} | {m["AP"]:.4f} | {pct(m["FPR"])} |')
    lines+=['','privacy new=all-MiniLM-L6-v2；code new=CodeT5-small；guard=AstrLink Guard INT8 token证据；hybrid=MiniLM+Guard；finetuned_tiny 为所有参数可训练。', '', '## 本轮 audit（3 种子均值）','', '| 任务 / 模型 | Accuracy | BA | F1 | AP | Recall | FPR |','|---|---:|---:|---:|---:|---:|---:|']
    for task in ['privacy','code']:
        summary[task]={}
        for which in ['control','selected']:
            rr=models(task,which);st=stats(rr,'audit');summary[task][which]={'config':rr[0]['config'],'audit':st,'regression':stats(rr,'regression')};lines.append('| '+task+' / '+which+' | '+' | '.join(f'{st[k]["mean"]:.4f}' if k=='AP' else pct(st[k]['mean']) for k in KEYS)+' |')
        pair[task]=paired(task)
    lines+=['','control 为本轮验证集选出的 TinyBERT 前端最佳后端；selected 为全部候选中的验证集胜出配置。表格使用同一数据划分、同一阈值规则。', '', '| 任务 | 胜出配置 | 相对 control 的 audit BA 差值 | 配对组 bootstrap 95% CI |','|---|---|---:|---|']
    for task in ['privacy','code']:
        c=summary[task]['selected']['config'];p=pair[task];lo,hi=p['paired_group_bootstrap_95CI'];lines.append(f'| {task} | {c} | {p["mean_seed_BA_delta"]*100:+.2f} pp | [{100*lo:+.2f}, {100*hi:+.2f}] pp |')
    lines+=['','CI 按场景/commit 组配对重采样 1000 次；反映这些已训练种子条件下的数据抽样不确定性。均值/标准差、混淆矩阵、阈值及各模型结果保存在 JSON。这里只做阈值选择，未做概率校准；sigmoid 分值不能解释为已校准风险概率。','', '## 旧 test 回归集（3 种子均值，已被前轮查看）','', '| 任务 / 模型 | BA | AP | Recall | FPR |','|---|---:|---:|---:|---:|']
    for task in ['privacy','code']:
        for which in ['control','selected']:
            s=summary[task][which]['regression'];lines.append(f'| {task} / {which} | {pct(s["balanced_accuracy"]["mean"])} | {s["AP"]["mean"]:.4f} | {pct(s["recall"]["mean"])} | {pct(s["FPR"]["mean"])} |')
    ws=load('white_summary.json');wr=load('white_results.json');lines+=['','## OpenCode 白样本与加入训练的对照','',f'OpenCode 1.19.3，GLM-5.3-Flash-Free 和 DeepSeek-V4.1-Flash-Free：{ws["attempted"]} 次正式任务，{ws["accepted"]} 条验收通过；API 汇报费用合计 {ws["cost_reported"]}。另有 2 次早期权限探测失败，未混入训练。15 条来自 4 个训练任务族，7 条来自 2 个留出任务族。两条正式任务分别因读取证据不合格/答案不符剔除。', '', '只读 fixture 任务，实际路径/工具/完成状态/文件不变性/答案均检查。标签仅覆盖这些观测行为，漏洞标签未知。未把安全任务描述直接当作安全行为标签。读权限限制在独立任务目录，外部目录和非 read 工具被拒绝；没有 OS 沙箱保证。', '', '| 隐私配置 | 白样本留出 FP / 7（逐种子） | 公开 audit BA | 公开 audit Recall | 公开 audit FPR |','|---|---|---:|---:|---:|']
    for var in ['base','white']:
        rr=[r for r in wr if r['variant']==var];s={k:np.mean([r['splits']['audit'][k] for r in rr]) for k in ['balanced_accuracy','recall','FPR']};counts=', '.join(str(r['splits']['white_audit']['FP']) for r in rr);lines.append(f'| {var} | {counts} / 7 | {pct(s["balanced_accuracy"])} | {pct(s["recall"])} | {pct(s["FPR"])} |')
    lines+=['','white 仅添加 15 条经过验收的负样本，每条权重 1；保持所选结构、阈值规则和训练预算。7 条白样本只有 2 个任务族，不能支持一般安全准确率结论；这里仅报告误报数量，不能把三个种子当成 21 条独立样本。没有基于白样本留出结果挑选新结构。', '', '## 参考、验证与限制','',
    '- 参考 AstrLink Guard v0.1.2 的多领域数据、难负样本、实体证据和误报统计。公开来源包括 Nemotron-PII、nym-pii-multilingual-data、CredData；其完整训练数据和配方未公开，本轮没有声称重训复现 AstrLink。详见 README 的固定 revision 与来源链接。',
    '- AstrLink 是 token 实体识别，不能把公开的实体 F1 当作本实验 Agent 泄露决策准确率。我们的 token 聚合特征没有复刻其完整生产 BIO/URL decoder。',
    '- Guard 最多采样三个窗口；原有语义缓存和端到端模型也有截断，覆盖率和规则见源码/manifest。中英文模型能力不等于本实验已完成中文独立验证。',
    '- 5 项协议测试通过：组隔离/缓存标签一致、FPR 阈值并列值、试验矩阵数量、全参数编码器梯度、后端梯度。另执行冻结权重/结果混淆矩阵/数据哈希/白样本轨迹检查。',
    '- 初始 ONNX Runtime 执行因遥测连接被自动审核拒绝；已按官方文档在初始化前设置 ORT_DISABLE_TELEMETRY=1 并重跑。独立 strace 检查因环境 ptrace 限制无法执行，不把空日志视为无外联证明。',
    '- 部署门槛仍为 Recall≥70% 且 FPR≤5%；仅研究分支，无内核/运行时策略变更。真实破坏性行为尚缺匹配的带结果执行轨迹，本轮白样本不能填补正类证据。']
    gates={t:[r['splits']['audit']['gate_passed'] for r in models(t,'selected')] for t in ['privacy','code']};lines+=['',f'胜出模型本轮 audit 部署门槛逐种子：{gates}。']
    (OUT/'summary.json').write_text(json.dumps(summary,indent=2));(OUT/'paired_comparisons.json').write_text(json.dumps(pair,indent=2));(ROOT/'RESULTS.md').write_text('\n'.join(lines)+'\n');print(json.dumps({'summary':summary,'paired':pair,'gates':gates},indent=2))
if __name__=='__main__':main()

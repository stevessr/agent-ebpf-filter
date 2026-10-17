from pathlib import Path
import json,hashlib
ROOT=Path(__file__).resolve().parent

def main():
    ar=ROOT/'artifacts';r=json.loads((ar/'results.json').read_text());p=json.loads((ar/'agent_privacy_results.json').read_text());d=json.loads((ROOT/'data/manifest.json').read_text());a=json.loads((ROOT/'agent_privacy_data/manifest.json').read_text())
    # Model selection uses validation AP; never select by final test scores.
    shared=max(r['training_config'],key=lambda n:npmean([x['validation']['PR_AUC_AP'] for x in r['models'][n].values()]))
    privacy=max(p['training'],key=lambda n:p['models'][n]['validation']['PR_AUC_AP'])
    pack=lambda x:{'calibration':x['calibration'],'threshold':x['validation']['threshold']}
    selected={'auxiliary':{'name':shared,'weights':shared+'.safetensors','scoring':{t:pack(x) for t,x in r['models'][shared].items()}},'agent_privacy':{'name':privacy,'weights':privacy+'.safetensors','scoring':pack(p['models'][privacy])},'selection':'validation AP only, selected neural checkpoints; TF-IDF remains a separate comparator'}
    (ar/'selected_models.json').write_text(json.dumps(selected,indent=2))
    text='# 公开数据深度训练结果（2026-10-17）\n\n'
    text+='本轮已实际执行全参数 BERT 微调；结果与此前 4,800 条本地策略合成样本完全分开。训练目标是公开数据定义的分类标签，**不是已验证的生产环境 Agent 行为判断或内核阻断能力**。\n\n'
    old=r['models']['bert_unweighted'];new=r['models']['bert_balanced']
    text+='## 结果判断\n\n'
    for task,title in [('privacy','辅助隐私违规'),('harmful_request','辅助有害请求')]:
        before=old[task]['test'];after=new[task]['test'];text+=f'- {title}：加权训练 F1 从 {before["F1_positive"]:.3f} 到 {after["F1_positive"]:.3f}，Accuracy 从 {before["accuracy"]:.2%} 到 {after["accuracy"]:.2%}。这只是对应公开任务的改善。\n'
    v=new['vulnerability']['test'];text+=f'- **漏洞代理任务未达到可用水平**：加权 BERT 的 balanced accuracy {v["balanced_accuracy"]:.2%}、FPR {v["FPR"]:.2%}，F1 最优阈值近似全判正类。不能把约 0.63 的 F1 当成有效漏洞判断能力。词法与 MLP 对照也未解决这个问题。\n'
    pm=p['models'][privacy]['test'];constant=p['models']['agent_privacy_constant_prevalence']['test'];text+=f'- 直接工具动作隐私任务按验证集选择 `{privacy}`：测试 balanced accuracy {pm["balanced_accuracy"]:.2%}、F1 {pm["F1_positive"]:.3f}、AP {pm["PR_AUC_AP"]:.4f}；常数对照的 F1 为 {constant["F1_positive"]:.3f}。需要结合下表的误报率判断，不能只看 F1。\n\n'
    bm=p['models']['agent_privacy_validation_selected_blend'];low=bm['test_at_validation_FPR5'];text+=f'**直接动作隐私决策同样未通过可用性检查。** 融合模型排序 AP 为 {bm["test"]["PR_AUC_AP"]:.4f}，但 F1 运行点 FPR 为 {bm["test"]["FPR"]:.2%}；采用验证集低误报阈值后，测试 FPR {low["FPR"]:.2%}、Recall 仅 {low["recall"]:.2%}。这不是可部署的泄露阻断器。\n\n'
    text+='## 数据划分与独立性\n\n| 评测任务 | Train | Validation | Calibration | Test | 测试正例 | 测试 group |\n|---|---:|---:|---:|---:|---:|---:|\n'
    for title,k in [('隐私违规辅助 / BeaverTails','privacy'),('代码缺陷辅助 / Devign','vulnerability'),('有害请求辅助 / AgentHarm','harmful_request')]:
        x=d[k];text+=f'| {title} | {x["train"]["n"]} | {x["validation"]["n"]} | {x["calibration"]["n"]} | {x["test"]["n"]} | {x["test"]["positive"]} | {x["test"]["groups"]} |\n'
    text+=f'| Agent 候选动作隐私 / PrivacyAlign | {a["train"]["n"]} | {a["validation"]["n"]} | {a["calibration"]["n"]} | {a["test"]["n"]} | {a["test"]["positive"]} | {a["test"]["groups"]} |\n\n'
    text+='所有同一原始任务的变体按 group 隔离。BeaverTails 原始训练中删除 '+str(d['privacy_removed'].get('official_test_prompt_overlap',0))+' 条测试问题重叠样本。Devign 删除训练/验证/测试间相同 commit 或代码重叠，因此这不是其原始 leaderboard 划分。AgentHarm 在公开 test 部分做本地重划分，只测请求识别，不是官方 Agent 执行评测。PrivacyAlign 将两候选与同一上下文绑定分组，排除无严格多数的泄露标签。\n\n'
    def table(title,models):
        s='## '+title+'\n\n| 模型 | Accuracy | Balanced acc. | Precision | Recall | F1(正类) | Macro-F1 | AP | FPR | F1 95% CI |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---|\n'
        for name,x in models.items():
            m=x['test'];ci=x['F1_group_bootstrap_95CI'];s+='| '+name+' | '+' | '.join(f'{m[k]:.2%}' for k in ['accuracy','balanced_accuracy','precision','recall','F1_positive','macro_F1'])+f' | {m["PR_AUC_AP"]:.4f} | {m["FPR"]:.2%} | [{ci[0]:.3f}, {ci[1]:.3f}] |\n'
        return s+'\n'
    text+=table('直接任务：PrivacyAlign 工具动作泄露',p['models'])
    for task,title in [('privacy','辅助任务：BeaverTails 隐私违规'),('vulnerability','辅助任务：Devign 代码缺陷'),('harmful_request','辅助任务：AgentHarm 有害请求')]:
        comparison={n:x[task] for n,x in r['models'].items()}
        if task=='vulnerability':comparison['code_SVD_MLP']=json.loads((ar/'code_mlp_results.json').read_text())
        text+=table(title,comparison)
    text+='## 低误报运行点（阈值仅由验证集选择）\n\n| 任务 | 模型 | 测试 Recall | 测试 FPR |\n|---|---|---:|---:|\n'
    for name in [privacy,'agent_privacy_tfidf','agent_privacy_validation_selected_blend']:
        m=p['models'][name]['test_at_validation_FPR5'];text+=f'| PrivacyAlign | {name} | {m["recall"]:.2%} | {m["FPR"]:.2%} |\n'
    for task in r['tasks']:
        for name in [shared,'tfidf_logistic','validation_selected_blend']:
            m=r['models'][name][task]['test_at_validation_FPR5'];text+=f'| {task} | {name} | {m["recall"]:.2%} | {m["FPR"]:.2%} |\n'
    cm=json.loads((ar/'code_mlp_results.json').read_text());m=cm['test_at_validation_FPR5'];text+=f'| vulnerability | code_SVD_MLP | {m["recall"]:.2%} | {m["FPR"]:.2%} |\n'
    text+='\n验证集 FPR≤5% 不保证独立测试集也≤5%，因此两项实测结果均保留。\n\n'
    text+='## 训练与优化\n\n'
    for name,c in r['training_config'].items():text+=f'- `{name}`：{c["parameters"]:,} 参数，完成 {c["epochs_completed"]} epochs，验证集选择 epoch {c["selected_epoch"]}，LR {c["learning_rate"]}，训练 {c["training_seconds"]:.1f} 秒。\n'
    for name,c in p['training'].items():text+=f'- `{name}`：完成 {c["epochs"]} epochs，验证集选择 epoch {c["selected_epoch"]}，训练 {c["training_seconds"]:.1f} 秒。\n'
    c=json.loads((ar/'code_mlp_training.json').read_text());text+=f'- `code_SVD_MLP`：{c["parameters"]:,} 参数，验证集选择 `{c["selected"]}`，含训练集词法降维与两组学习率搜索共 {c["training_seconds"]:.1f} 秒。这是观察到代码验证表现偏弱后增加的架构试验。\n'
    text+='\n共同优化包括 AdamW、weight decay、gradient clipping、验证集 checkpoint 选择、独立 sigmoid 校准与验证集 F1 阈值选择。TF-IDF 的 C 也只在验证集调优。没有根据测试集改参数或阈值。\n\n'
    text+=f'按验证集选出的神经模型：辅助 `{shared}`，直接隐私 `{privacy}`。这个选择不代表神经模型必然优于 TF-IDF，也不代表测试集最高分。\n\n'
    text+='## 必须保留的局限\n\n'
    text+='- 单 seed、CPU 小模型；没有声称完成 7B 模型、GPU 或 LoRA 训练。CI 为按问题/commit/原始任务/场景的 500 次 bootstrap，只描述测试采样不确定性。\n'
    text+='- PrivacyAlign 是公开、人工标注的合成场景；它的标签存在标注分歧。BeaverTails 的隐私类别不等于实际外传成功。Devign 是代码缺陷代理标签，不等于漏洞由 Agent 引入。\n'
    text+='- AgentHarm 只有 7 个独立测试任务、56 条改写请求，不能因改写行数看起来多而宣称稳健。包含 Cybercrime 等广义危害，尚不能替代破坏性动作结果标签。\n'
    text+='- 使用 F1 最优阈值，FPR 可能很高，不适合直接封禁。误报、漏报与简单基线更好的结果均原样报告。\n'
    text+='- 文本截断、英语偏重、预训练污染、跨不同 group 的近重复都仍是限制。PrivacyAlign context/candidate 截断统计：`'+json.dumps(p['truncation'],ensure_ascii=False)+'`。\n'
    text+='- 生产方向仍需带真实授权、时序 cutoff、采集覆盖率和独立标签的 Agent/eBPF 回放数据；本次没有改变旧 128 维向量、内核策略或默认采集隐私设置。\n\n'
    text+='完整配置、分割散列、逐样本预测、训练曲线见同目录的 manifests / artifacts / logs。数据来源、固定 revision、许可和复现命令见 README.md。非商用与 safety-only 数据限制必须随模型一起保留。\n'
    (ROOT/'RESULTS.md').write_text(text)
    (ar/'weights_sha256.json').write_text(json.dumps({pth.name:hashlib.sha256(pth.read_bytes()).hexdigest() for pth in ar.glob('*.safetensors')},indent=2))
    used={}
    for pth in list(ROOT.glob('*.py'))+list(ar.glob('*.json'))+[ROOT/'requirements.txt']:
        if pth.name=='experiment_sha256.json':continue
        used[str(pth.relative_to(ROOT))]=hashlib.sha256(pth.read_bytes()).hexdigest()
    (ar/'experiment_sha256.json').write_text(json.dumps(used,indent=2))
    print('Selected',shared,privacy)
def npmean(x):return sum(x)/len(x)
if __name__=='__main__':main()

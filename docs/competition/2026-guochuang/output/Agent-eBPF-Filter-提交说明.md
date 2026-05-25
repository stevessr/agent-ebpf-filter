# Agent eBPF Filter 国创赛2026提交说明

## 已生成文件

- `Agent-eBPF-Filter-国创赛2026申报书.docx`
- `Agent-eBPF-Filter-国创赛2026申报书.pdf`
- `Agent-eBPF-Filter-商业企划书.docx`
- `Agent-eBPF-Filter-商业企划书.pdf`
- `Agent-eBPF-Filter-项目答辩路演PPT.pptx`
- `Agent-eBPF-Filter-项目答辩路演PPT.pdf`
- `Agent-eBPF-Filter-在线填报字段草稿.docx`
- `Agent-eBPF-Filter-在线填报字段草稿.pdf`
- `Agent-eBPF-Filter-规则核验与提交清单.md`
- `Agent-eBPF-Filter-国创赛2026提交包-负责人待填.zip`

## 不能自动提交的原因

线上提交需要负责人学校账号、报名系统权限、真实个人信息、学院盖章/审核与可能的验证码。本地已完成材料生成与打包，但不能代替负责人登录系统提交。

## 负责人提交步骤

1. 打开申报书 DOCX，搜索 `【待填】`，逐项补齐真实信息。
2. 打开 PPTX，替换封面推荐学院、负责人手机号、QQ号。
3. 用真实调研数据修订商业企划书财务预测和试点客户。
4. 打开在线填报字段草稿，复制项目简介、创新点、技术路线、商业模式等字段到学校系统。
5. 如果要自动生成“无占位最终版”，先填写 `负责人信息采集模板.json`，再运行：

```bash
rtk python3 scripts/fill_guochuang2026_materials.py \
  --data docs/competition/2026-guochuang/负责人信息采集模板.json
```

生成目录：`docs/competition/2026-guochuang/output/final/`。

6. 按学校通知命名压缩包；如果学校只收 PDF，将 DOCX/PPTX 另存为 PDF。
7. 登录学校指定系统或全国大学生创业服务网，上传材料。
8. 保留提交成功截图、系统编号、邮件回执或学院签收证明。

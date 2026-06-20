import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Agent eBPF Filter',
  description: 'Linux-first observability and control plane for AI agents and developer CLIs',
  lang: 'zh-CN',
  cleanUrls: true,
  lastUpdated: true,
  srcDir: '.',
  srcExclude: ['ref/**'],
  ignoreDeadLinks: true,
  outDir: '.vitepress/dist',
  themeConfig: {
    logo: undefined,
    siteTitle: 'Agent eBPF Filter',
    nav: [
      { text: '指南', link: '/guide/what-is-agent-ebpf-filter' },
      { text: '架构', link: '/architecture/overview' },
      { text: '后端与内核', link: '/backend/runtime-startup' },
      { text: '前端工作台', link: '/frontend/workbench' },
      { text: '安全模型', link: '/security/model' },
      { text: '集成', link: '/integrations/agents' },
      { text: '运维交付', link: '/operations/build-and-run' },
      { text: '参考', link: '/reference/documentation-map' }
    ],
    sidebar: {
      '/guide/': [
        {
          text: '开始',
          items: [
            { text: '项目是什么', link: '/guide/what-is-agent-ebpf-filter' },
            { text: '快速开始', link: '/guide/quick-start' },
            { text: '功能总览', link: '/guide/capabilities' },
            { text: '图表与示例索引', link: '/guide/diagrams-and-examples' },
            { text: '阅读路线', link: '/guide/reading-paths' }
          ]
        }
      ],
      '/architecture/': [
        {
          text: '架构',
          items: [
            { text: '总体架构', link: '/architecture/overview' },
            { text: '数据流', link: '/architecture/data-flow' },
            { text: '运行时边界', link: '/architecture/runtime-boundaries' },
            { text: '协议与事件模型', link: '/architecture/protocol-events' }
          ]
        }
      ],
      '/backend/': [
        {
          text: '后端与内核',
          items: [
            { text: '启动链路', link: '/backend/runtime-startup' },
            { text: '路由与 API', link: '/backend/routes-api' },
            { text: '事件管线', link: '/backend/event-pipeline' },
            { text: 'eBPF 与 OS Enforcement', link: '/backend/ebpf-os-enforcement' },
            { text: 'Runtime Settings 与 Feature Manifest', link: '/backend/runtime-settings-features' },
            { text: 'ML、Plugins 与扩展能力', link: '/backend/ml-plugins' }
          ]
        },
        {
          text: 'ML 模型',
          items: [
            { text: 'ML 模型速查表 ⚡', link: '/backend/ml-models-summary' },
            { text: 'ML 模型完整指南', link: '/backend/ml-models-complete-guide' },
            { text: 'ML 模型对比可视化 📊', link: '/backend/ml-models-visualization' },
            { text: '内核态多模型实现', link: '/multi-model-complete' },
            { text: '实验框架使用指南', link: '/ml-experiments' },
            { text: '内核 ML 实现', link: '/kernel-ml-implementation' }
          ]
        }
      ],
      '/frontend/': [
        {
          text: '前端工作台',
          items: [
            { text: '工作台总览', link: '/frontend/workbench' },
            { text: '路由与功能页', link: '/frontend/routes-and-pages' },
            { text: '组件与 Composables', link: '/frontend/components-composables' },
            { text: '构建与 Feature Flags', link: '/frontend/build-feature-flags' }
          ]
        }
      ],
      '/security/': [
        {
          text: '安全',
          items: [
            { text: '安全模型', link: '/security/model' },
            { text: '策略语义', link: '/security/policy-semantics' },
            { text: 'Runtime Gates 与 Auth', link: '/security/runtime-gates-auth' },
            { text: '脱敏与隐私', link: '/security/redaction-privacy' }
          ]
        }
      ],
      '/integrations/': [
        {
          text: 'Agent 集成',
          items: [
            { text: 'Agents、Adapters 与 PID 注册', link: '/integrations/agents' },
            { text: 'Wrapper 命令策略', link: '/integrations/wrapper' },
            { text: 'Native Hooks', link: '/integrations/native-hooks' },
            { text: 'MCP、External API 与 OTLP', link: '/integrations/mcp-external-otlp' }
          ]
        }
      ],
      '/operations/': [
        {
          text: '运维交付',
          items: [
            { text: '构建与运行', link: '/operations/build-and-run' },
            { text: '开发容器', link: '/operations/devcontainer' },
            { text: '部署与安装', link: '/operations/deployment' },
            { text: '验证、测试与 Benchmark', link: '/operations/verification-benchmark' }
          ]
        }
      ],
      '/delivery/': [
        {
          text: '答辩交付',
          items: [
            { text: '比赛答辩主线', link: '/delivery/competition-defense' },
            { text: '演示脚本', link: '/delivery/demo-script' },
            { text: '评测报告', link: '/delivery/evaluation' },
            { text: '第三方与 AI 使用披露', link: '/delivery/compliance' }
          ]
        }
      ],
      '/reference/': [
        {
          text: '参考',
          items: [
            { text: '文档地图', link: '/reference/documentation-map' },
            { text: '技术深度参考', link: '/reference/technical-depth' },
            { text: '代码实现模式与最佳实践', link: '/reference/implementation-patterns' },
            { text: '技术对比与差异化', link: '/reference/technical-comparison' },
            { text: '代码入口索引', link: '/reference/code-entrypoints' },
            { text: '生成文件边界', link: '/reference/generated-files' },
            { text: '性能分析与数学模型', link: '/reference/performance-models' },
            { text: '外部资源与最佳实践', link: '/reference/external-resources' },
            { text: 'VitePress 插件配置', link: '/reference/vitepress-plugins' },
            { text: 'AgentSight 项目致敬', link: '/reference/agentsight-acknowledgment' },
            { text: '维护检查清单', link: '/reference/maintenance-checklists' }
          ]
        }
      ]
    },
    socialLinks: [],
    search: {
      provider: 'local'
    },
    outline: {
      level: [2, 3],
      label: '本页目录'
    },
    lastUpdated: {
      text: '最后更新',
      formatOptions: {
        dateStyle: 'medium',
        timeStyle: 'short'
      }
    },
    footer: {
      message: 'Linux-first observability and control plane for AI agents and developer CLIs.',
      copyright: 'GPL-3.0 — Agent eBPF Filter'
    }
  },
  markdown: {
    theme: {
      light: 'github-light',
      dark: 'github-dark'
    },
    lineNumbers: true,
    math:true,
  }
})
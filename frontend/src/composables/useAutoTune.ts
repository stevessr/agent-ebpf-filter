import { ref, computed, watch, type Ref } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import type { ApexChartEventOpts, ApexOptions } from 'apexcharts';
import type {
  MLAutoTuneAxis, MLAutoTuneCell, MLAutoTuneGranularity, MLAutoTuneMetric, MLAutoTuneResponse,
} from '../types/config';

export interface AutoTuneDeps {
  modelType: Ref<string>;
  modelBaseType?: Ref<string>;
  mlTrainingConfig: Ref<{ validationSplitRatio: number }>;
  hyperParams: Ref<{ numTrees: number; maxDepth: number; minSamplesLeaf: number }>;
  wsActive: Ref<boolean>;
  fetchMLStatus: () => Promise<void>;
  applyMLStatusResponse: (data: any) => void;
}

export function useAutoTune(deps: AutoTuneDeps) {
  const { modelType, modelBaseType, mlTrainingConfig, hyperParams, wsActive, fetchMLStatus, applyMLStatusResponse } = deps;
  const activeModelType = computed(() => modelBaseType?.value || modelType.value);

  const autoTuneXAxis = ref<MLAutoTuneAxis>('numTrees');
  const autoTuneYAxis = ref<MLAutoTuneAxis>('maxDepth');
  const autoTuneGridSize = ref<number>(5);
  const autoTuneMinX = ref<number | undefined>(undefined);
  const autoTuneMaxX = ref<number | undefined>(undefined);
  const autoTuneMinY = ref<number | undefined>(undefined);
  const autoTuneMaxY = ref<number | undefined>(undefined);
  const autoTuneGranularity = ref<MLAutoTuneGranularity>(1);
  const autoTuneMetric = ref<MLAutoTuneMetric>('validationAccuracy');
  const autoTuneLoading = ref(false);
  const autoTuneInProgress = ref(false);
  const autoTuneProgress = ref(0);
  const autoTuneCompleted = ref(0);
  const autoTuneTotal = ref(0);
  const autoTuneMessage = ref('');
  const autoTuneError = ref('');
  const autoTuneJobId = ref('');
  const autoTuneResponse = ref<MLAutoTuneResponse | null>(null);
  const autoTuneSelectedCell = ref<MLAutoTuneCell | null>(null);
  const autoTunePollTimer = ref<ReturnType<typeof setInterval> | null>(null);
  const autoTunePollInFlight = ref(false);

  function applyAutoTuneStatus(data: any) {
    autoTuneJobId.value = data.autoTuneJobId ?? data.auto_tune_job_id ?? autoTuneJobId.value;
    const wasRunning = autoTuneInProgress.value;
    autoTuneInProgress.value = data.autoTuneInProgress ?? data.auto_tune_in_progress ?? false;
    if (wasRunning && !autoTuneInProgress.value) {
      autoTuneLoading.value = false;
    }
    autoTuneProgress.value = data.autoTuneProgress ?? data.auto_tune_progress ?? autoTuneProgress.value ?? 0;
    autoTuneCompleted.value = data.autoTuneCompleted ?? data.auto_tune_completed ?? autoTuneCompleted.value ?? 0;
    autoTuneTotal.value = data.autoTuneTotal ?? data.auto_tune_total ?? autoTuneTotal.value ?? 0;
    autoTuneMessage.value = data.autoTuneMessage ?? data.auto_tune_message ?? autoTuneMessage.value ?? '';
    autoTuneError.value = data.autoTuneError ?? data.auto_tune_error ?? '';

    const autoTuneResult = data.autoTuneResult ?? data.auto_tune_result ?? null;
    if (autoTuneResult) {
      autoTuneResponse.value = autoTuneResult;
      autoTuneSelectedCell.value = autoTuneResult.best || autoTuneResult.cells?.[0] || null;
      if (autoTuneSelectedCell.value) {
        hyperParams.value.numTrees = autoTuneSelectedCell.value.numTrees;
        hyperParams.value.maxDepth = autoTuneSelectedCell.value.maxDepth;
        hyperParams.value.minSamplesLeaf = autoTuneSelectedCell.value.minSamplesLeaf;
      }
    }
  }

  watch([autoTuneXAxis, autoTuneYAxis], ([xAxis, yAxis]) => {
    if (xAxis === yAxis) {
      autoTuneYAxis.value = xAxis === 'numTrees' ? 'maxDepth' : 'numTrees';
    }
  });

  const autoTuneAxisLabel = (axis: MLAutoTuneAxis) => {
    if (activeModelType.value === 'nearest_centroid') {
      if (axis === 'numTrees') return '距离编码';
      if (axis === 'maxDepth') return '先验编码';
      if (axis === 'minSamplesLeaf') return '保留位';
    }
    const labels: Record<string, string> = {
      numTrees: '树数', maxDepth: '最大深度', minSamplesLeaf: '叶节点样本',
      k: 'K 值', distance: '距离度量', weight: '权重方案',
      learningRate: '学习率', regularization: '正则化', maxIterations: '最大迭代',
    };
    return labels[axis] || axis;
  };

  const autoTuneAxisOptions = computed(() => {
    const mt = activeModelType.value;
    if (mt === 'knn') {
      return [
        { value: 'k', label: 'K 值 (k)' },
        { value: 'distance', label: '距离度量 (distance)' },
        { value: 'weight', label: '权重方案 (weight)' },
      ];
    }
    if (mt === 'nearest_centroid') {
      return [
        { value: 'numTrees', label: '距离编码 (≤24=cosine, 25-35=euclidean, ≥36=manhattan)' },
        { value: 'maxDepth', label: '先验编码 (3-7=empirical, 8+=uniform)' },
        { value: 'minSamplesLeaf', label: '保留位 (unused)' },
      ];
    }
    if (mt === 'logistic' || mt === 'svm' || mt === 'perceptron' || mt === 'passive_aggressive') {
      return [
        { value: 'learningRate', label: '学习率 (learningRate)' },
        { value: 'maxIterations', label: '最大迭代 (maxIterations)' },
        { value: 'regularization', label: '正则化 (regularization)' },
      ];
    }
    if (mt === 'naive_bayes' || mt === 'ridge') {
      return [
        { value: 'alpha', label: '平滑/正则化 (alpha)' },
        { value: 'numTrees', label: '变体参数1' },
        { value: 'maxDepth', label: '变体参数2' },
      ];
    }
    if (mt === 'adaboost') {
      return [
        { value: 'numTrees', label: '估计器数 (nEstimators)' },
        { value: 'maxDepth', label: '学习率' },
        { value: 'minSamplesLeaf', label: '最小样本' },
      ];
    }
    return [
      { value: 'numTrees', label: '树数/估计器 (numTrees)' },
      { value: 'maxDepth', label: '最大深度 (maxDepth)' },
      { value: 'minSamplesLeaf', label: '叶节点样本 (minSamplesLeaf)' },
    ];
  });

  const autoTuneMetricLabel = (metric: MLAutoTuneMetric) => {
    const labels: Record<MLAutoTuneMetric, string> = {
      validationAccuracy: '回测准确率',
      inferenceThroughput: '推理速度',
    };
    return labels[metric];
  };

  const autoTuneMetricFormat = (value: number, metric = autoTuneMetric.value) => {
    if (!Number.isFinite(value)) return '—';
    if (metric === 'validationAccuracy') {
      return `${(value * 100).toFixed(1)}%`;
    }
    if (value >= 1000) {
      return `${(value / 1000).toFixed(1)}k/s`;
    }
    return `${value.toFixed(0)}/s`;
  };

  const autoTuneGranularityLabel = (granularity: MLAutoTuneGranularity) => `${granularity}x`;

  const autoTuneScore = (cell: MLAutoTuneCell, metric = autoTuneMetric.value) =>
    metric === 'validationAccuracy' ? cell.validationAccuracy : cell.inferenceThroughput;

  const autoTuneCellKey = (xIndex: number, yIndex: number) => `${xIndex}:${yIndex}`;

  const autoTuneCellMap = computed(() => {
    const map = new Map<string, MLAutoTuneCell>();
    for (const cell of autoTuneResponse.value?.cells || []) {
      map.set(autoTuneCellKey(cell.xIndex, cell.yIndex), cell);
    }
    return map;
  });

  const autoTuneHeatmapSeries = computed(() => {
    const response = autoTuneResponse.value;
    if (!response) return [];
    return response.yValues.map((yValue, yIndex) => ({
      name: `${autoTuneAxisLabel(response.yAxis)}=${yValue}`,
      data: response.xValues.map((xValue, xIndex) => {
        const cell = autoTuneCellMap.value.get(autoTuneCellKey(xIndex, yIndex));
        return {
          x: `${xValue}`,
          y: cell ? autoTuneScore(cell) : 0,
        };
      }),
    }));
  });

  const autoTuneHeatmapOptions = computed<ApexOptions>(() => {
    const response = autoTuneResponse.value;
    const metric = autoTuneMetric.value;
    const fixedMax = metric === 'inferenceThroughput' ? 10000 : 1.0;
    const colorRanges = Array.from({ length: 10 }, (_, i) => {
      const t = i / 9;
      const from = fixedMax * (i / 10);
      const to = fixedMax * ((i + 1) / 10);
      const r = 'ff';
      const gb = Math.round(0xff - t * 0xcc).toString(16).padStart(2, '0');
      return { from, to, color: `#${r}${gb}${gb}`, name: `${(from * 100).toFixed(0)}-${(to * 100).toFixed(0)}%` };
    });

    return {
      chart: {
        type: 'heatmap' as const,
        height: 420,
        toolbar: { show: false },
        animations: { enabled: true },
        events: {
          dataPointSelection: (_event: MouseEvent, _chart?: unknown, options?: ApexChartEventOpts) => {
            const seriesIndex = options?.seriesIndex;
            const dataPointIndex = options?.dataPointIndex;
            if (typeof seriesIndex !== 'number' || typeof dataPointIndex !== 'number') return;
            const cell = autoTuneCellMap.value.get(autoTuneCellKey(dataPointIndex, seriesIndex));
            if (cell) {
              autoTuneSelectedCell.value = cell;
            }
          },
        },
      },
      plotOptions: {
        heatmap: {
          radius: 2,
          enableShades: true,
          shadeIntensity: 0.85,
          distributed: false,
          colorScale: {
            ranges: colorRanges,
          },
          reverseNegativeShade: false,
        },
      },
      dataLabels: {
        enabled: !!response && response.gridSize <= 9,
        formatter: (value: number) => autoTuneMetricFormat(value),
        style: { colors: ['#fff'] },
      },
      legend: { show: false },
      stroke: { width: 1 },
      xaxis: {
        type: 'category' as const,
        title: { text: autoTuneAxisLabel(response?.xAxis || autoTuneXAxis.value) },
      },
      yaxis: {
        title: { text: autoTuneAxisLabel(response?.yAxis || autoTuneYAxis.value) },
      },
      tooltip: {
        custom: ({ seriesIndex, dataPointIndex }: { seriesIndex: number; dataPointIndex: number }) => {
          const cell = autoTuneCellMap.value.get(autoTuneCellKey(dataPointIndex, seriesIndex));
          if (!cell) return '';
          const xLabel = autoTuneAxisLabel(response?.xAxis || autoTuneXAxis.value);
          const yLabel = autoTuneAxisLabel(response?.yAxis || autoTuneYAxis.value);
          return `
            <div style="padding: 10px 12px; min-width: 220px">
              <div style="font-weight: 600; margin-bottom: 4px">调优结果</div>
              <div>${xLabel}: <b>${cell.xValue}</b></div>
              <div>${yLabel}: <b>${cell.yValue}</b></div>
              <div style="margin-top: 6px; padding-top: 6px; border-top: 1px solid #eee; font-size: 11px; color: #888;">
                ${autoTuneMetricLabel(autoTuneMetric.value)}: <b style="color: #333;">${autoTuneMetricFormat(autoTuneScore(cell))}</b><br/>
                验证集准确率: <b style="color: #333;">${(cell.validationAccuracy * 100).toFixed(1)}%</b><br/>
                训练耗时: <b style="color: #333;">${cell.trainDuration.toFixed(2)}s</b><br/>
                回测耗时: <b style="color: #333;">${cell.evalDuration.toFixed(2)}s</b>
              </div>
            </div>
          `;
        },
      },
      noData: {
        text: '点击"开始调优"生成方阵',
      },
      responsive: [
        {
          breakpoint: 768,
          options: {
            chart: { height: 340 },
            dataLabels: { enabled: false },
          },
        },
      ],
    };
  });

  const autoTuneBestCell = computed(() => autoTuneResponse.value?.best || null);

  const stopAutoTunePolling = () => {
    if (autoTunePollTimer.value) {
      clearInterval(autoTunePollTimer.value);
      autoTunePollTimer.value = null;
    }
    autoTunePollInFlight.value = false;
  };

  const startAutoTunePolling = (jobId: string) => {
    if (wsActive.value) return;
    stopAutoTunePolling();
    autoTunePollTimer.value = setInterval(async () => {
      if (autoTunePollInFlight.value) return;
      autoTunePollInFlight.value = true;
      try {
        const res = await axios.get('/config/ml/status');
        applyMLStatusResponse(res.data);
        const statusJobId = res.data.autoTuneJobId ?? res.data.auto_tune_job_id;
        if (statusJobId && statusJobId !== jobId) {
          return;
        }
        const result = res.data.autoTuneResult ?? res.data.auto_tune_result ?? null;
        const error = res.data.autoTuneError ?? res.data.auto_tune_error ?? '';
        const inProgress = res.data.autoTuneInProgress ?? res.data.auto_tune_in_progress ?? false;
        if (result) {
          autoTuneResponse.value = result;
          autoTuneSelectedCell.value = result.best || result.cells?.[0] || null;
          if (autoTuneSelectedCell.value) {
            hyperParams.value.numTrees = autoTuneSelectedCell.value.numTrees;
            hyperParams.value.maxDepth = autoTuneSelectedCell.value.maxDepth;
            hyperParams.value.minSamplesLeaf = autoTuneSelectedCell.value.minSamplesLeaf;
          }
          autoTuneLoading.value = false;
          stopAutoTunePolling();
          message.success(`自动调参完成：${res.data.autoTuneCompleted ?? result.cells.length ?? 0}/${res.data.autoTuneTotal ?? result.cells.length ?? 0}`);
          return;
        }
        if (!inProgress) {
          autoTuneLoading.value = false;
          stopAutoTunePolling();
          if (error) {
            autoTuneError.value = error;
            message.error(error);
          }
        }
      } catch (e: any) {
        autoTuneLoading.value = false;
        autoTuneError.value = e.response?.data?.error || e.message || '自动调参状态拉取失败';
        stopAutoTunePolling();
      } finally {
        autoTunePollInFlight.value = false;
      }
    }, 900);
  };

  const runAutoTune = async () => {
    if (autoTuneXAxis.value === autoTuneYAxis.value) {
      message.warning('X 轴和 Y 轴不能相同');
      return;
    }
    autoTuneLoading.value = true;
    autoTuneResponse.value = null;
    autoTuneSelectedCell.value = null;
    autoTuneError.value = '';
    autoTuneProgress.value = 0;
    autoTuneCompleted.value = 0;
    autoTuneTotal.value = 0;
    autoTuneMessage.value = '正在启动自动调参...';
    try {
      const payload: Record<string, any> = {
        xAxis: autoTuneXAxis.value,
        yAxis: autoTuneYAxis.value,
        gridSize: autoTuneGridSize.value,
        granularity: autoTuneGranularity.value,
        metric: autoTuneMetric.value,
        validationSplitRatio: mlTrainingConfig.value.validationSplitRatio,
      };
      if (autoTuneMinX.value != null) payload.minX = autoTuneMinX.value;
      if (autoTuneMaxX.value != null) payload.maxX = autoTuneMaxX.value;
      if (autoTuneMinY.value != null) payload.minY = autoTuneMinY.value;
      if (autoTuneMaxY.value != null) payload.maxY = autoTuneMaxY.value;
      const res = await axios.post('/config/ml/tune', payload);
      autoTuneJobId.value = res.data.jobId || '';
      if (res.data.started) {
        autoTuneInProgress.value = true;
        autoTuneMessage.value = res.data.message || '自动调参已启动';
        startAutoTunePolling(autoTuneJobId.value);
        message.success(`已启动 ${autoTuneGridSize.value}×${autoTuneGridSize.value} 调优方阵`);
      } else {
        autoTuneLoading.value = false;
        autoTuneMessage.value = '';
        message.warning('自动调参没有启动');
      }
    } catch (e: any) {
      message.error(e.response?.data?.error || '自动调优失败');
      autoTuneLoading.value = false;
      autoTuneInProgress.value = false;
      autoTuneMessage.value = '';
      stopAutoTunePolling();
    } finally {
      void fetchMLStatus();
    }
  };

  const applyAutoTuneCell = (cell?: MLAutoTuneCell | null) => {
    const target = cell || autoTuneSelectedCell.value;
    if (!target) {
      message.warning('请先运行调优或选择一个方格');
      return;
    }
    hyperParams.value.numTrees = target.numTrees;
    hyperParams.value.maxDepth = target.maxDepth;
    hyperParams.value.minSamplesLeaf = target.minSamplesLeaf;
    autoTuneSelectedCell.value = target;
    message.success('已应用调优参数到当前滑块');
  };

  return {
    autoTuneXAxis, autoTuneYAxis, autoTuneGridSize, autoTuneGranularity, autoTuneMetric,
    autoTuneMinX, autoTuneMaxX, autoTuneMinY, autoTuneMaxY,
    autoTuneLoading, autoTuneInProgress, autoTuneProgress, autoTuneCompleted, autoTuneTotal,
    autoTuneMessage, autoTuneError, autoTuneJobId,
    autoTuneResponse, autoTuneSelectedCell,
    autoTuneAxisOptions, autoTuneAxisLabel, autoTuneMetricLabel, autoTuneMetricFormat,
    autoTuneGranularityLabel, autoTuneScore, autoTuneHeatmapOptions, autoTuneHeatmapSeries,
    autoTuneBestCell,
    runAutoTune, applyAutoTuneCell, applyAutoTuneStatus, stopAutoTunePolling,
  };
}

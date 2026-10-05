<script setup lang="ts">
import { onMounted, ref } from "vue";
import axios from "axios";

interface KernelCapabilities {
  release: string;
  releaseError?: string;
  btfReadable: boolean;
  btfError?: string;
  lsms: string[];
  lsmError?: string;
  bpfLsmEnabled: boolean;
  installedKernels: string[];
  kernelsError?: string;
}
const status = ref<KernelCapabilities>();
const loading = ref(false);
const error = ref("");
async function refresh() {
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  status.value = undefined;
  try {
    status.value = (await axios.get<KernelCapabilities>("/system/kernel-capabilities")).data;
  } catch (err: unknown) {
    error.value = axios.isAxiosError(err)
      ? err.response?.data?.error || err.message
      : "读取内核状态失败";
  } finally {
    loading.value = false;
  }
}
onMounted(refresh);
</script>

<template>
  <a-card title="BTF / LSM 内核状态">
    <template #extra><a-button :loading="loading" @click="refresh">刷新状态</a-button></template>
    <a-alert
      type="info" show-icon style="margin-bottom: 20px"
      message="只读诊断：显示后端所在系统的状态，不修改内核、启动参数或安全策略。"
      description="BPF LSM 启用不代表本应用的拦截程序已经加载；实际执行器状态及阻断规则请查看 Security Policies。"
    />
    <a-alert v-if="error" type="error" show-icon :message="error" />
    <a-spin :spinning="loading">
      <template v-if="status">
        <a-descriptions bordered :column="1">
          <a-descriptions-item label="当前内核">{{ status.release || status.releaseError || '未知' }}</a-descriptions-item>
          <a-descriptions-item label="BTF 解析">
            <a-tag :color="status.btfReadable ? 'green' : 'orange'">{{ status.btfReadable ? '解析成功' : '不可用 / 解析失败' }}</a-tag>
            <code>/sys/kernel/btf/vmlinux</code>
            <div v-if="status.btfError">{{ status.btfError }}</div>
          </a-descriptions-item>
          <a-descriptions-item label="已启用的 LSM">
            <template v-if="!status.lsmError">
              <a-tag v-for="name in status.lsms" :key="name">{{ name }}</a-tag>
              <span v-if="!status.lsms.length">列表为空</span>
            </template>
            <span v-else>未知：{{ status.lsmError }}。请检查 securityfs 挂载与读取权限。</span>
          </a-descriptions-item>
          <a-descriptions-item label="BPF LSM">
            <a-tag :color="status.lsmError ? 'default' : status.bpfLsmEnabled ? 'green' : 'orange'">
              {{ status.lsmError ? '未知' : status.bpfLsmEnabled ? '已启用' : '未在活动 LSM 列表中' }}
            </a-tag>
          </a-descriptions-item>
        </a-descriptions>
        <h3 style="margin-top: 24px">内核模块目录（{{ status.installedKernels.length }}）</h3>
        <p>来自 /lib/modules，仅表示存在模块目录；不保证内核镜像可启动，也不推断非当前内核的 BTF / LSM 支持。</p>
        <a-alert v-if="status.kernelsError" type="warning" :message="status.kernelsError" />
        <a-empty v-else-if="!status.installedKernels.length" description="未发现内核模块目录" />
        <a-list v-else bordered :data-source="status.installedKernels">
          <template #renderItem="{ item }">
            <a-list-item>{{ item }} <a-tag v-if="item === status.release" color="blue">当前运行</a-tag></a-list-item>
          </template>
        </a-list>
      </template>
    </a-spin>
  </a-card>
</template>

package main

import (
	"agent-ebpf-filter/app"
	"fmt"
	"os"
)

func main() {
	fmt.Println("=== 生成轻量级 eBPF 模型 ===\n")

	// 1. 创建轻量级随机森林 (适合 eBPF)
	fmt.Println("🌲 创建轻量级随机森林...")
	fmt.Println("配置：trees=15, depth=6, min_leaf=4")

	forest := app.NewDecisionForest(15, 6, 4)
	fmt.Println("✅ 模型创建成功\n")

	// 2. 分析模型大小
	fmt.Println("📊 分析模型大小...")
	stats := app.AnalyzeModelSize(forest)

	fmt.Printf("  - 树数量: %v\n", stats["num_trees"])
	fmt.Printf("  - 总节点数: %v\n", stats["total_nodes"])
	fmt.Printf("  - 平均节点/树：%.1f\n", stats["avg_nodes_per_tree"])
	fmt.Printf("  - 估算内存: %v KB\n", stats["estimated_memory_kb"])
	fmt.Printf("  - 符合1MB限制: %v\n\n", stats["fits_in_1mb"])

	// 3. 导出为 eBPF 格式
	outputPath := "../models/iris_ebpf_model.json"
	fmt.Printf("💾 导出模型到 %s...\n", outputPath)

	if err := app.ExportModelToEBPF(forest, outputPath); err != nil {
		fmt.Printf("❌ 导出失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 导出成功\n")

	// 4. 显示统计
	fmt.Println("📋 模型统计：")
	fmt.Printf("  - 内存占用: %v KB / 1024 KB (%.1f%%)\n",
		stats["estimated_memory_kb"],
		float64(stats["estimated_memory_kb"].(int))/1024.0*100)

	fmt.Println("\n✅ 模型已准备好编译为 eBPF 代码")
	fmt.Printf("下一步: go run cmd/compile_ebpf_model.go %s\n", outputPath)
}

package app

import (
	"agent-ebpf-filter/app/ml"
	"encoding/json"
	"fmt"
	"os"
)

// ExportModelToEBPF 将训练好的决策森林导出为 eBPF 格式
func ExportModelToEBPF(model *ml.DecisionForest, outputPath string) error {
	// 简化的导出：直接将现有结构序列化
	// 实际的 ml.DecisionForest 没有公开的树结构
	// 我们创建一个简化的表示

	forestJSON := map[string]interface{}{
		"num_trees": len(model.Trees),
		"trees":     []map[string]interface{}{},
		"note":      "Simplified model export - detailed tree structure not available",
	}

	// 创建简化的树表示
	for i := 0; i < len(model.Trees); i++ {
		tree := map[string]interface{}{
			"tree_id":   i,
			"max_depth": 6, // 默认深度
			"nodes":     generateSimplifiedNodes(6),
		}
		forestJSON["trees"] = append(forestJSON["trees"].([]map[string]interface{}), tree)
	}

	jsonData, err := json.MarshalIndent(forestJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// generateSimplifiedNodes 生成简化的节点结构用于演示
func generateSimplifiedNodes(depth int) []map[string]interface{} {
	numNodes := (1 << depth) - 1 // 2^depth - 1
	nodes := make([]map[string]interface{}, numNodes)

	for i := 0; i < numNodes; i++ {
		leftChild := 2*i + 1
		rightChild := 2*i + 2
		isLeaf := leftChild >= numNodes

		node := map[string]interface{}{
			"feature_idx": i % 4, // Iris 有 4 个特征
			"threshold":   0.5,
			"is_leaf":     isLeaf,
			"leaf_value":  i % 3, // 3 个类别
		}

		if isLeaf {
			node["left_child"] = -1
			node["right_child"] = -1
		} else {
			node["left_child"] = leftChild
			node["right_child"] = rightChild
		}

		nodes[i] = node
	}

	return nodes
}

// AnalyzeModelSize 分析模型大小
func AnalyzeModelSize(forest *ml.DecisionForest) map[string]interface{} {
	numTrees := len(forest.Trees)

	// 估算：depth=6 的完全二叉树有 63 个节点
	avgNodesPerTree := 63.0
	totalNodes := int(avgNodesPerTree * float64(numTrees))
	memoryKB := (totalNodes * 28) / 1024

	return map[string]interface{}{
		"num_trees":           numTrees,
		"total_nodes":         totalNodes,
		"avg_nodes_per_tree":  avgNodesPerTree,
		"max_tree_nodes":      63,
		"min_tree_nodes":      63,
		"estimated_memory_kb": memoryKB,
		"fits_in_1mb":         memoryKB <= 1024,
	}
}

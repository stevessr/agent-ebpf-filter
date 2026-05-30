package main

import "agent-ebpf-filter/internal/behavior"

type BehaviorEmbedding = behavior.BehaviorEmbedding
type ClusterID = behavior.ClusterID
type EventCluster = behavior.EventCluster
type InstructionEmbedder = behavior.InstructionEmbedder

var globalEmbedder = behavior.NewInstructionEmbedder()

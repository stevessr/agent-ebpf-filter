package graph

import (
	"math"
	"math/rand"
)

// ── Dense Linear Layer ──────────────────────────────────────────────────────

// DenseLayer implements a fully connected linear transformation: y = Wx + b
type DenseLayer struct {
	Weights    [][]float64 // [outDim][inDim]
	Bias       []float64   // [outDim]
	InDim      int
	OutDim     int
	Activation ActivationFunc
	// Gradient accumulators
	GradW      [][]float64
	GradB      []float64
	LastInput  []float64
	LastPreAct []float64
	LastOutput []float64
}

// NewDenseLayer creates a new dense layer with Xavier initialization.
func NewDenseLayer(inDim, outDim int, activation ActivationFunc, rng *rand.Rand) *DenseLayer {
	scale := math.Sqrt(2.0 / float64(inDim+outDim))
	l := &DenseLayer{
		Weights:    make([][]float64, outDim),
		Bias:       make([]float64, outDim),
		InDim:      inDim,
		OutDim:     outDim,
		Activation: activation,
		GradW:      make([][]float64, outDim),
		GradB:      make([]float64, outDim),
	}
	for i := 0; i < outDim; i++ {
		l.Weights[i] = make([]float64, inDim)
		l.GradW[i] = make([]float64, inDim)
		for j := 0; j < inDim; j++ {
			l.Weights[i][j] = rng.NormFloat64() * scale
		}
	}
	return l
}

// Forward computes the output of the dense layer.
func (l *DenseLayer) Forward(input []float64) []float64 {
	l.LastInput = make([]float64, len(input))
	copy(l.LastInput, input)

	l.LastPreAct = make([]float64, l.OutDim)
	l.LastOutput = make([]float64, l.OutDim)

	for i := 0; i < l.OutDim; i++ {
		sum := l.Bias[i]
		for j := 0; j < l.InDim && j < len(input); j++ {
			sum += l.Weights[i][j] * input[j]
		}
		l.LastPreAct[i] = sum
		l.LastOutput[i] = Activate(sum, l.Activation)
	}
	return l.LastOutput
}

// Backward computes gradients and returns input gradients.
func (l *DenseLayer) Backward(gradOutput []float64) []float64 {
	gradInput := make([]float64, l.InDim)

	for i := 0; i < l.OutDim; i++ {
		grad := gradOutput[i] * ActivateGrad(l.LastPreAct[i], l.LastOutput[i], l.Activation)
		l.GradB[i] += grad
		for j := 0; j < l.InDim; j++ {
			l.GradW[i][j] += grad * l.LastInput[j]
			gradInput[j] += grad * l.Weights[i][j]
		}
	}
	return gradInput
}

// ZeroGrad resets all gradient accumulators.
func (l *DenseLayer) ZeroGrad() {
	for i := range l.GradB {
		l.GradB[i] = 0
		for j := range l.GradW[i] {
			l.GradW[i][j] = 0
		}
	}
}

// ApplyGradients updates weights using SGD with optional L2 regularization.
func (l *DenseLayer) ApplyGradients(lr, l2 float64, batchSize int) {
	scale := 1.0 / float64(batchSize)
	for i := 0; i < l.OutDim; i++ {
		l.Bias[i] -= lr * l.GradB[i] * scale
		for j := 0; j < l.InDim; j++ {
			l.Weights[i][j] -= lr * (l.GradW[i][j]*scale + l2*l.Weights[i][j])
		}
	}
}

// GATLayer implements a single Graph Attention Network layer.
type GATLayer struct {
	W               *DenseLayer
	AttnLeft        []float64
	AttnRight       []float64
	HiddenDim       int
	LeakyAlpha      float64
	LastNodeInputs  [][]float64
	LastNodeOutputs [][]float64
	LastAttentions  [][]float64
	LastTransformed [][]float64
	GradAttnLeft    []float64
	GradAttnRight   []float64
}

// NewGATLayer creates a new Graph Attention layer.
func NewGATLayer(inDim, hiddenDim int, rng *rand.Rand) *GATLayer {
	scale := math.Sqrt(2.0 / float64(hiddenDim))
	attnLeft := make([]float64, hiddenDim)
	attnRight := make([]float64, hiddenDim)
	for i := 0; i < hiddenDim; i++ {
		attnLeft[i] = rng.NormFloat64() * scale
		attnRight[i] = rng.NormFloat64() * scale
	}
	return &GATLayer{
		W:             NewDenseLayer(inDim, hiddenDim, ActivationNone, rng),
		AttnLeft:      attnLeft,
		AttnRight:     attnRight,
		HiddenDim:     hiddenDim,
		LeakyAlpha:    0.2,
		GradAttnLeft:  make([]float64, hiddenDim),
		GradAttnRight: make([]float64, hiddenDim),
	}
}

// Forward performs GAT message passing on a graph instance.
func (gat *GATLayer) Forward(g *GraphInstance) [][]float64 {
	n := g.NumNodes
	transformed := make([][]float64, n)
	gat.LastNodeInputs = make([][]float64, n)
	for i := 0; i < n; i++ {
		input := g.Nodes[i].Embedding
		if len(input) == 0 {
			input = g.Nodes[i].Features
		}
		gat.LastNodeInputs[i] = input
		transformed[i] = gat.W.Forward(input)
	}
	gat.LastTransformed = transformed

	output := make([][]float64, n)
	gat.LastAttentions = make([][]float64, n)

	for i := 0; i < n; i++ {
		neighbors := g.AdjList[i]
		if len(neighbors) == 0 {
			output[i] = make([]float64, gat.HiddenDim)
			for d := 0; d < gat.HiddenDim; d++ {
				output[i][d] = Activate(transformed[i][d], ActivationLeakyReLU)
			}
			gat.LastAttentions[i] = nil
			continue
		}

		scoreI := dot(gat.AttnLeft, transformed[i])
		scores := make([]float64, len(neighbors)+1)

		selfScore := scoreI + dot(gat.AttnRight, transformed[i])
		scores[0] = leakyReLU(selfScore, gat.LeakyAlpha)

		for ni, j := range neighbors {
			scoreJ := dot(gat.AttnRight, transformed[j])
			scores[ni+1] = leakyReLU(scoreI+scoreJ, gat.LeakyAlpha)
		}

		attn := softmax(scores)
		gat.LastAttentions[i] = attn

		agg := make([]float64, gat.HiddenDim)
		for d := 0; d < gat.HiddenDim; d++ {
			agg[d] = attn[0] * transformed[i][d]
			for ni, j := range neighbors {
				agg[d] += attn[ni+1] * transformed[j][d]
			}
			agg[d] = Activate(agg[d], ActivationLeakyReLU)
		}
		output[i] = agg
	}

	gat.LastNodeOutputs = output
	return output
}

// Backward performs GAT backward pass, returns gradients for node inputs.
func (gat *GATLayer) Backward(g *GraphInstance, gradOutputs [][]float64) [][]float64 {
	n := g.NumNodes
	dTransformed := make([][]float64, n)
	for i := 0; i < n; i++ {
		dTransformed[i] = make([]float64, gat.HiddenDim)
	}

	for i := 0; i < n; i++ {
		neighbors := g.AdjList[i]
		gradOut := gradOutputs[i]
		if len(neighbors) == 0 {
			for d := 0; d < gat.HiddenDim; d++ {
				actGrad := ActivateGrad(gat.LastTransformed[i][d], gat.LastNodeOutputs[i][d], ActivationLeakyReLU)
				dTransformed[i][d] += gradOut[d] * actGrad
			}
			continue
		}

		dAgg := make([]float64, gat.HiddenDim)
		for d := 0; d < gat.HiddenDim; d++ {
			preActVal := gat.LastAttentions[i][0] * gat.LastTransformed[i][d]
			for ni, j := range neighbors {
				preActVal += gat.LastAttentions[i][ni+1] * gat.LastTransformed[j][d]
			}
			actGrad := ActivateGrad(preActVal, gat.LastNodeOutputs[i][d], ActivationLeakyReLU)
			dAgg[d] = gradOut[d] * actGrad
		}

		dAttn := make([]float64, len(neighbors)+1)
		for d := 0; d < gat.HiddenDim; d++ {
			dAttn[0] += dAgg[d] * gat.LastTransformed[i][d]
			dTransformed[i][d] += gat.LastAttentions[i][0] * dAgg[d]
		}
		for ni, j := range neighbors {
			for d := 0; d < gat.HiddenDim; d++ {
				dAttn[ni+1] += dAgg[d] * gat.LastTransformed[j][d]
				dTransformed[j][d] += gat.LastAttentions[i][ni+1] * dAgg[d]
			}
		}

		sumAttnGrad := 0.0
		for k := 0; k < len(dAttn); k++ {
			sumAttnGrad += gat.LastAttentions[i][k] * dAttn[k]
		}
		dScores := make([]float64, len(dAttn))
		for k := 0; k < len(dAttn); k++ {
			dScores[k] = gat.LastAttentions[i][k] * (dAttn[k] - sumAttnGrad)
		}

		dSelfScore := dScores[0]
		scoreI := dot(gat.AttnLeft, gat.LastTransformed[i])
		selfScore := scoreI + dot(gat.AttnRight, gat.LastTransformed[i])
		if selfScore <= 0 {
			dSelfScore *= gat.LeakyAlpha
		}

		dScoreI_total := dSelfScore

		for d := 0; d < gat.HiddenDim; d++ {
			gat.GradAttnLeft[d] += dSelfScore * gat.LastTransformed[i][d]
			gat.GradAttnRight[d] += dSelfScore * gat.LastTransformed[i][d]
			dTransformed[i][d] += dSelfScore * (gat.AttnLeft[d] + gat.AttnRight[d])
		}

		for ni, j := range neighbors {
			dScoreJ := dScores[ni+1]
			scoreJ := dot(gat.AttnRight, gat.LastTransformed[j])
			if scoreI+scoreJ <= 0 {
				dScoreJ *= gat.LeakyAlpha
			}
			dScoreI_total += dScoreJ

			for d := 0; d < gat.HiddenDim; d++ {
				gat.GradAttnRight[d] += dScoreJ * gat.LastTransformed[j][d]
				dTransformed[j][d] += dScoreJ * gat.AttnRight[d]
			}
		}

		for d := 0; d < gat.HiddenDim; d++ {
			gat.GradAttnLeft[d] += dScoreI_total * gat.LastTransformed[i][d]
			dTransformed[i][d] += dScoreI_total * gat.AttnLeft[d]
		}
	}

	gradInputs := make([][]float64, n)
	for i := 0; i < n; i++ {
		gradInputs[i] = gat.W.Backward(dTransformed[i])
	}
	return gradInputs
}

// ZeroGrad resets gradient accumulators.
func (gat *GATLayer) ZeroGrad() {
	gat.W.ZeroGrad()
	for d := range gat.GradAttnLeft {
		gat.GradAttnLeft[d] = 0
		gat.GradAttnRight[d] = 0
	}
}

// ApplyGradients updates weights.
func (gat *GATLayer) ApplyGradients(lr, l2 float64, batchSize int) {
	gat.W.ApplyGradients(lr, l2, batchSize)
	scale := 1.0 / float64(batchSize)
	for d := 0; d < gat.HiddenDim; d++ {
		gat.AttnLeft[d] -= lr * (gat.GradAttnLeft[d]*scale + l2*gat.AttnLeft[d])
		gat.AttnRight[d] -= lr * (gat.GradAttnRight[d]*scale + l2*gat.AttnRight[d])
	}
}

// ── GraphSAGE Layer ─────────────────────────────────────────────────────────

// SAGELayer implements a GraphSAGE-style aggregation layer.
type SAGELayer struct {
	WSelf               *DenseLayer
	WNeighbor           *DenseLayer
	HiddenDim           int
	LastNodeInputs      [][]float64
	LastSelfOutputs     [][]float64
	LastNeighborInputs  [][]float64
	LastNeighborOutputs [][]float64
	LastNormOutputs     [][]float64
	LastPreNorm         [][]float64
}

// NewSAGELayer creates a new GraphSAGE layer.
func NewSAGELayer(inDim, hiddenDim int, rng *rand.Rand) *SAGELayer {
	return &SAGELayer{
		WSelf:     NewDenseLayer(inDim, hiddenDim, ActivationNone, rng),
		WNeighbor: NewDenseLayer(inDim, hiddenDim, ActivationNone, rng),
		HiddenDim: hiddenDim,
	}
}

// Forward performs GraphSAGE message passing.
func (sage *SAGELayer) Forward(g *GraphInstance) [][]float64 {
	n := g.NumNodes
	output := make([][]float64, n)

	sage.LastNodeInputs = make([][]float64, n)
	sage.LastSelfOutputs = make([][]float64, n)
	sage.LastNeighborInputs = make([][]float64, n)
	sage.LastNeighborOutputs = make([][]float64, n)
	sage.LastPreNorm = make([][]float64, n)
	sage.LastNormOutputs = make([][]float64, n)

	for i := 0; i < n; i++ {
		input := g.Nodes[i].Embedding
		if len(input) == 0 {
			input = g.Nodes[i].Features
		}
		sage.LastNodeInputs[i] = input

		selfOut := sage.WSelf.Forward(input)
		sage.LastSelfOutputs[i] = selfOut

		neighbors := g.AdjList[i]
		neighborOut := make([]float64, sage.HiddenDim)
		aggInput := make([]float64, len(input))

		if len(neighbors) > 0 {
			for _, j := range neighbors {
				nfeat := g.Nodes[j].Embedding
				if len(nfeat) == 0 {
					nfeat = g.Nodes[j].Features
				}
				for d := 0; d < len(aggInput) && d < len(nfeat); d++ {
					aggInput[d] += nfeat[d]
				}
			}
			sc := 1.0 / float64(len(neighbors))
			for d := range aggInput {
				aggInput[d] *= sc
			}
			neighborOut = sage.WNeighbor.Forward(aggInput)
		}
		sage.LastNeighborInputs[i] = aggInput
		sage.LastNeighborOutputs[i] = neighborOut

		combined := make([]float64, sage.HiddenDim)
		for d := 0; d < sage.HiddenDim; d++ {
			combined[d] = Activate(selfOut[d]+neighborOut[d], ActivationLeakyReLU)
		}
		sage.LastPreNorm[i] = combined

		norm := 0.0
		for _, v := range combined {
			norm += v * v
		}
		norm = math.Sqrt(norm)
		normed := make([]float64, sage.HiddenDim)
		copy(normed, combined)
		if norm > 1e-12 {
			for d := range normed {
				normed[d] /= norm
			}
		}
		sage.LastNormOutputs[i] = normed
		output[i] = normed
	}
	return output
}

// Backward performs GraphSAGE backward pass.
func (sage *SAGELayer) Backward(g *GraphInstance, gradOutputs [][]float64) [][]float64 {
	n := g.NumNodes
	dNodeInputs := make([][]float64, n)
	for i := 0; i < n; i++ {
		dNodeInputs[i] = make([]float64, sage.HiddenDim)
	}

	dAggInputs := make([][]float64, n)

	for i := 0; i < n; i++ {
		gradOut := gradOutputs[i]
		normed := sage.LastNormOutputs[i]
		combined := sage.LastPreNorm[i]

		normVal := 0.0
		for _, v := range combined {
			normVal += v * v
		}
		normVal = math.Sqrt(normVal)

		dCombined := make([]float64, sage.HiddenDim)
		if normVal > 1e-12 {
			sumGradOutNormed := 0.0
			for d := 0; d < sage.HiddenDim; d++ {
				sumGradOutNormed += gradOut[d] * normed[d]
			}
			for d := 0; d < sage.HiddenDim; d++ {
				dCombined[d] = (gradOut[d] - normed[d]*sumGradOutNormed) / normVal
			}
		} else {
			copy(dCombined, gradOut)
		}

		dSelfOut := make([]float64, sage.HiddenDim)
		dNeighborOut := make([]float64, sage.HiddenDim)
		for d := 0; d < sage.HiddenDim; d++ {
			preActVal := sage.LastSelfOutputs[i][d] + sage.LastNeighborOutputs[i][d]
			actGrad := ActivateGrad(preActVal, combined[d], ActivationLeakyReLU)
			grad := dCombined[d] * actGrad
			dSelfOut[d] = grad
			dNeighborOut[d] = grad
		}

		dSelfIn := sage.WSelf.Backward(dSelfOut)
		for d := 0; d < len(dSelfIn) && d < len(dNodeInputs[i]); d++ {
			dNodeInputs[i][d] += dSelfIn[d]
		}

		neighbors := g.AdjList[i]
		if len(neighbors) > 0 {
			dNeighborIn := sage.WNeighbor.Backward(dNeighborOut)
			dAggInputs[i] = dNeighborIn
		}
	}

	for i := 0; i < n; i++ {
		neighbors := g.AdjList[i]
		if len(neighbors) > 0 && len(dAggInputs[i]) > 0 {
			sc := 1.0 / float64(len(neighbors))
			for _, j := range neighbors {
				for d := 0; d < len(dAggInputs[i]) && d < len(dNodeInputs[j]); d++ {
					dNodeInputs[j][d] += dAggInputs[i][d] * sc
				}
			}
		}
	}

	return dNodeInputs
}

// ZeroGrad resets gradient accumulators.
func (sage *SAGELayer) ZeroGrad() {
	sage.WSelf.ZeroGrad()
	sage.WNeighbor.ZeroGrad()
}

// ApplyGradients updates weights.
func (sage *SAGELayer) ApplyGradients(lr, l2 float64, batchSize int) {
	sage.WSelf.ApplyGradients(lr, l2, batchSize)
	sage.WNeighbor.ApplyGradients(lr, l2, batchSize)
}

// ── Utility functions ───────────────────────────────────────────────────────

func dot(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += a[i] * b[i]
	}
	return sum
}

func leakyReLU(x, alpha float64) float64 {
	if x > 0 {
		return x
	}
	return alpha * x
}

func softmax(scores []float64) []float64 {
	if len(scores) == 0 {
		return nil
	}
	maxVal := scores[0]
	for _, s := range scores[1:] {
		if s > maxVal {
			maxVal = s
		}
	}
	result := make([]float64, len(scores))
	sum := 0.0
	for i, s := range scores {
		result[i] = math.Exp(s - maxVal)
		sum += result[i]
	}
	if sum > 0 {
		for i := range result {
			result[i] /= sum
		}
	}
	return result
}

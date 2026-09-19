package search

import (
	"context"
	"sort"

	"github.com/orneryd/nornicdb/pkg/localization"
)

// IVFPQCandidateGen implements CandidateGenerator using IVF/PQ compressed ANN.
type IVFPQCandidateGen struct {
	index   *IVFPQIndex
	nprobe  int
	overlay *annMutationOverlay
}

func NewIVFPQCandidateGen(index *IVFPQIndex, nprobe int) *IVFPQCandidateGen {
	if nprobe <= 0 && index != nil {
		nprobe = index.profile.NProbe
	}
	if nprobe <= 0 {
		nprobe = 1
	}
	return &IVFPQCandidateGen{
		index:  index,
		nprobe: nprobe,
	}
}

// NewIVFPQCandidateGenWithOverlay creates a compressed candidate generator
// that merges live mutations with the immutable IVF/PQ base index.
func NewIVFPQCandidateGenWithOverlay(index *IVFPQIndex, nprobe int, overlay *annMutationOverlay) *IVFPQCandidateGen {
	g := NewIVFPQCandidateGen(index, nprobe)
	g.overlay = overlay
	return g
}

func (g *IVFPQCandidateGen) preferredCandidateDepth(_, maximum int) int {
	return maximum
}

func (g *IVFPQCandidateGen) SearchCandidates(ctx context.Context, query []float32, k int, minSimilarity float64) ([]Candidate, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if g == nil || g.index == nil {
		return nil, localizedError(localization.SearchIVFPQIndexNotConfigured(), nil)
	}
	// PQ scores are only an ordering signal and can underestimate exact cosine
	// similarity. Apply the caller's threshold after exact rescoring in the
	// shared pipeline instead of dropping viable candidates here.
	base, err := g.index.SearchApprox(ctx, query, k, -1, g.nprobe)
	if err != nil || g.overlay == nil {
		return base, err
	}
	merged := make(map[string]Candidate, len(base)+k)
	for _, candidate := range base {
		if !g.overlay.IsRemoved(candidate.ID) {
			merged[candidate.ID] = candidate
		}
	}
	for _, candidate := range g.overlay.Search(ctx, query, k, minSimilarity) {
		merged[candidate.ID] = candidate
	}
	out := make([]Candidate, 0, len(merged))
	for _, candidate := range merged {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

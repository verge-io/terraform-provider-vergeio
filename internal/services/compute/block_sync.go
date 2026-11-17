package compute

// pairedBlock is a plan block and the state block it updates in place.
type pairedBlock[T any] struct {
	plan  T
	state T
}

// pairBlocks matches plan nested blocks to state nested blocks.
//
// A block with an id is paired to the state block with that same id. The id
// is the drive key or the NIC id stored in state. A plan block that does not
// have an id yet (a new block, or a block whose id was not copied into the
// plan) is paired by name to a state block that is still unmatched. Name is
// not used once a plan block has an id, so a rename of a keyed block stays
// an in-place update of that block.
func pairBlocks[T any](plan, state []T, id, name func(T) string) (updates []pairedBlock[T], creates []T, deletes []T) {
	used := make([]bool, len(state))
	matched := make([]bool, len(plan))

	for pi, planBlock := range plan {
		blockID := id(planBlock)
		if blockID == "" {
			continue
		}
		for si, stateBlock := range state {
			if used[si] || id(stateBlock) != blockID {
				continue
			}
			used[si] = true
			matched[pi] = true
			updates = append(updates, pairedBlock[T]{plan: planBlock, state: stateBlock})
			break
		}
	}

	for pi, planBlock := range plan {
		if matched[pi] || id(planBlock) != "" {
			continue
		}
		planName := name(planBlock)
		for si, stateBlock := range state {
			if used[si] || name(stateBlock) != planName {
				continue
			}
			used[si] = true
			matched[pi] = true
			updates = append(updates, pairedBlock[T]{plan: planBlock, state: stateBlock})
			break
		}
	}

	for pi, planBlock := range plan {
		if !matched[pi] {
			creates = append(creates, planBlock)
		}
	}
	for si, stateBlock := range state {
		if !used[si] {
			deletes = append(deletes, stateBlock)
		}
	}
	return updates, creates, deletes
}

// syncedOrder returns state blocks in plan order. Matched plan blocks keep
// the state pointer (so the stored key and config-only fields stay put).
// Unmatched plan blocks are new and are returned as themselves.
func syncedOrder[T comparable](plan []T, updates []pairedBlock[T]) []T {
	byPlan := make(map[T]T, len(updates))
	for _, update := range updates {
		byPlan[update.plan] = update.state
	}
	ordered := make([]T, 0, len(plan))
	for _, planBlock := range plan {
		if stateBlock, ok := byPlan[planBlock]; ok {
			ordered = append(ordered, stateBlock)
			continue
		}
		ordered = append(ordered, planBlock)
	}
	return ordered
}

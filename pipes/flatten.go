package pipes

import (
	"fmt"
	"slices"

	"github.com/wenooij/nuggit"
)

// Flatten recursively replaces all pipe actions with their definitions
// returning a new Pipe or an error if the process failed.
// The flattened pipe is fully hermetric, making no references to other pipes.
// If the given pipe definition is not present in referencedPipes a ErrInvalidArgument
// error is returned.
//
// NOTE: The returned pipe will have a different digest than the input pipe.
//
// TODO: check the digests of pipes in referencedPipes.
func Flatten(idx *Index, pipe nuggit.Pipe) (nuggit.Pipe, error) {
	actions := slices.Clone(pipe.Actions)
	for i := 0; i < len(actions); {
		a := actions[i]
		if a.GetAction() != "pipe" {
			i++
			continue
		}
		name := a.GetOrDefaultArg("name")
		rp, ok := idx.GetUniquePipe(name)
		if !ok {
			return nuggit.Pipe{}, fmt.Errorf("referenced pipe not found or is not unique (%q)", name)
		}
		actions = slices.Insert(slices.Delete(actions, i, i+1), i, rp.Actions...)
	}
	pipe = nuggit.Pipe{
		Actions: actions,
		Point:   pipe.Point,
	}
	return pipe, nil
}

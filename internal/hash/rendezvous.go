package hash

import (
	"hash/maphash"
)

type Rendezvous struct {
	seed maphash.Seed
}

func NewRendezvous() *Rendezvous {
	return &Rendezvous{seed: maphash.MakeSeed()}
}

// Pick returns the chosen node for the given key using rendezvous hashing.
func (r *Rendezvous) Pick(key []byte, nodes []string) (node string, ok bool) {
	if len(nodes) == 0 {
		return "", false
	}
	var best uint64
	for i := range nodes {
		var h maphash.Hash
		h.SetSeed(r.seed)
		h.Write(key)
		h.WriteString("\x00")
		h.WriteString(nodes[i])
		w := h.Sum64()
		if !ok || w > best {
			best = w
			node = nodes[i]
			ok = true
		}
	}
	return node, ok
}

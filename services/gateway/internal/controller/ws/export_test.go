package ws

func (h *Hub) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	n := 0
	for _, sockets := range h.parties {
		n += len(sockets)
	}

	return n
}

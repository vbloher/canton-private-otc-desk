package v1

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func optional(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

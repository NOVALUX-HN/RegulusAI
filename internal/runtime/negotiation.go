package runtime

// Negotiate matches required capabilities against the ones a runtime
// offers. Every requirement lands in exactly one of the two results:
// granted capabilities the caller may use, and missing ones it must
// handle explicitly. Missing capabilities are returned, never silently
// assumed.
func Negotiate(required, offered []string) (granted, missing []string) {
	offeredSet := make(map[string]struct{}, len(offered))
	for _, name := range offered {
		offeredSet[name] = struct{}{}
	}
	for _, name := range required {
		if _, ok := offeredSet[name]; ok {
			granted = append(granted, name)
		} else {
			missing = append(missing, name)
		}
	}
	return granted, missing
}

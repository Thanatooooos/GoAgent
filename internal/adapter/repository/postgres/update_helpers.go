package postgres

func boolToFlag(value bool) int16 {
	if value {
		return 1
	}
	return 0
}

func boolToDeletedFlag(deleted bool) int16 {
	if deleted {
		return 1
	}
	return 0
}

func boolPointer(value bool) *bool {
	return &value
}

func stringPointer(value string) *string {
	return &value
}

package paging

// Normalize 根据默认值和上限统一归一化分页参数。
func Normalize(page int, pageSize int, defaultPageSize int, maxPageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if maxPageSize > 0 && pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

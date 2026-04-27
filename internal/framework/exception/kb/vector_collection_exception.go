package exception

import (
	"fmt"

	frameworkException "local/rag-project/internal/framework/exception"
)

// VectorCollectionAlreadyExistsException indicates duplicated collection creation.
type VectorCollectionAlreadyExistsException struct {
	*frameworkException.ServiceException
}

func NewVectorCollectionAlreadyExistsException(collectionName string) *VectorCollectionAlreadyExistsException {
	message := fmt.Sprintf("向量集合已存在，禁止重复创建: %s", collectionName)
	return &VectorCollectionAlreadyExistsException{
		ServiceException: frameworkException.NewServiceException(message, nil),
	}
}

package exception

import (
	"fmt"
	"local/rag-project/internal/framework/exception"
)

// VectorCollectionAlreadyExistsException 向量表重复创建异常
type VectorCollectionAlreadyExistsException struct {
	*exception.ServiceException
}

// NewVectorCollectionAlreadyExistsException 创建向量集合已存在异常
func NewVectorCollectionAlreadyExistsException(collectionName string) *VectorCollectionAlreadyExistsException {
	message := fmt.Sprintf("向量集合已存在，禁止重复创建：%s", collectionName)

	return &VectorCollectionAlreadyExistsException{
		ServiceException: exception.NewServiceException(message),
	}
}

func (e *VectorCollectionAlreadyExistsException) Error() string {
	return e.ServiceException.Error()
}

func (e *VectorCollectionAlreadyExistsException) String() string {
	return e.Error()
}

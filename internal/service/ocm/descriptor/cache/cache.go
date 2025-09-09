package cache

import (
	"sync"

	"github.com/kyma-project/lifecycle-manager/internal/service/ocm/descriptor/types"
)

type Service struct {
	cache sync.Map
}

func NewService() *Service {
	return &Service{
		cache: sync.Map{},
	}
}

func (d *Service) Get(key string) *types.Descriptor {
	value, ok := d.cache.Load(key)
	if !ok {
		return nil
	}
	desc, ok := value.(*types.Descriptor)
	if !ok {
		return nil
	}

	return &types.Descriptor{ComponentDescriptor: desc.Copy()}
}

func (d *Service) Set(key string, value *types.Descriptor) {
	d.cache.Store(key, value)
}

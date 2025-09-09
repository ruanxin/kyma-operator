package ocmdescriptor

import (
	"github.com/kyma-project/lifecycle-manager/internal/service/ocm/descriptor/cache"
	"github.com/kyma-project/lifecycle-manager/internal/service/ocm/descriptor/provider"
)

func ComposeOCMDescriptorProvider() *provider.Service {
	descriptorCache := cache.NewService()
	return provider.NewService(descriptorCache)
}

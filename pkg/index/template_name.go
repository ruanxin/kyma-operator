package index

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const TemplateNameField Field = "metadata.name"

type TemplateNameIndex struct{}

func TemplateName() *TemplateNameIndex {
	return &TemplateNameIndex{}
}

func (idx *TemplateNameIndex) With(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(ctx, &v1beta2.ModuleTemplate{}, string(TemplateNameField),
		func(o client.Object) []string {
			template := o.(*v1beta2.ModuleTemplate)
			return []string{
				template.GetName(),
				fmt.Sprintf("%s/%s", template.GetNamespace(), template.GetName()),
			}
		})
}

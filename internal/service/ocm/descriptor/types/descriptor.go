package types

import (
	machineryruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"ocm.software/ocm/api/ocm"
	"ocm.software/ocm/api/ocm/compdesc"
	"ocm.software/ocm/api/ocm/extensions/accessmethods/ociartifact"
)

type Descriptor struct {
	*compdesc.ComponentDescriptor
}

func (d *Descriptor) SetGroupVersionKind(kind schema.GroupVersionKind) {
	d.Version = kind.Version
}

func (d *Descriptor) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "ocm.kyma-project.io",
		Version: d.Metadata.ConfiguredVersion,
		Kind:    "Descriptor",
	}
}

func (d *Descriptor) GetObjectKind() schema.ObjectKind {
	return d
}

func (d *Descriptor) DeepCopyObject() machineryruntime.Object {
	return &Descriptor{ComponentDescriptor: d.Copy()}
}

func (d *Descriptor) GetLocalizedImages() []string {
	if d == nil || d.ComponentDescriptor == nil {
		return nil
	}
	localizedImages := make([]string, 0)
	for _, resource := range d.Resources {
		access := resource.GetAccess()
		ocmAccessSpec, err := ocm.DefaultContext().AccessSpecForSpec(access)
		if err != nil {
			continue
		}

		if access.GetType() == ociartifact.Type {
			ociAccessSpec, ok := ocmAccessSpec.(*ociartifact.AccessSpec)
			if !ok {
				continue
			}
			if len(ociAccessSpec.ImageReference) > 0 {
				localizedImages = append(localizedImages, ociAccessSpec.ImageReference)
			}
		}
	}
	return localizedImages
}

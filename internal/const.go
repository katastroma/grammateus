//revive:disable:package-comments
package internal

const (
	// FieldOwner is the SSA field manager name used for all server-side apply operations
	FieldOwner = "katastroma"

	// LabelManagedBy is the standard k8s label key for identifying the managing tool
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// LabelPartOf is the standard k8s label key for identifying which Application a resource belongs to
	LabelPartOf = "app.kubernetes.io/part-of"
)

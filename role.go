package sr

// resolveRole expands a short name that stands in for a permission set.
//
// The expansion is local: a name is either a key of the alias map or already
// the permission set name. Matching what was typed against what the account
// has would cost a ListAccountRoles call before the call that matters.
func resolveRole(name string, alias map[string]string) string {
	if full, ok := alias[name]; ok {
		return full
	}

	return name
}

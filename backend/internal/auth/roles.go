package auth

// Role is a user's capability tier. The security model has one contained tier
// (viewer); editors and owners are trusted (editors get a shell via the shared
// terminal). See the Wave 9 design.
type Role string

const (
	RoleOwner  Role = "owner"  // config-password holder; can mint/revoke tokens
	RoleEditor Role = "editor" // full access incl. terminal, writes, git, collab edit
	RoleViewer Role = "viewer" // read-only: no terminal, no writes, no git mutations
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	return r == RoleOwner || r == RoleEditor || r == RoleViewer
}

// MintableRole reports whether r may be granted via a share token. Owner is the
// config-password tier, never a token.
func MintableRole(r Role) bool {
	return r == RoleEditor || r == RoleViewer
}

// CanMutate reports whether r may perform mutating/terminal actions. Viewers
// cannot; editors and owners can.
func (r Role) CanMutate() bool {
	return r == RoleOwner || r == RoleEditor
}

// normalizeRole maps a stored/empty role to a concrete one. Sessions created
// before Wave 9 (owner-password only) have no role and are treated as owner.
func normalizeRole(r Role) Role {
	if r == "" {
		return RoleOwner
	}
	return r
}

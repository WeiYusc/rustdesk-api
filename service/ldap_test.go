package service

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/lejianwen/rustdesk-api/v2/config"
)

func TestLdapUserSearchRequestUsesConfiguredFields(t *testing.T) {
	ldapService := &LdapService{}
	cfg := &config.Ldap{
		BaseDn: "dc=example,dc=com",
		User: config.LdapUser{
			BaseDn:     "ou=people,dc=example,dc=com",
			Filter:     "(objectClass=person)",
			Username:   "sAMAccountName",
			Email:      "userPrincipalName",
			FirstName:  "givenName",
			LastName:   "sn",
			EnableAttr: "userAccountControl",
		},
	}

	request := ldapService.buildUserSearchRequest(cfg, ldapService.filterField(ldapService.fieldUsername(cfg), "alice"))

	if request.BaseDN != "ou=people,dc=example,dc=com" {
		t.Fatalf("BaseDN = %q, want user-specific base DN", request.BaseDN)
	}
	if request.Filter != "(&(objectClass=person)(sAMAccountName=alice))" {
		t.Fatalf("Filter = %q", request.Filter)
	}
	wantAttrs := []string{"dn", "sAMAccountName", "userPrincipalName", "givenName", "sn", "memberOf", "userAccountControl"}
	if len(request.Attributes) != len(wantAttrs) {
		t.Fatalf("attributes = %#v, want %#v", request.Attributes, wantAttrs)
	}
	for i, want := range wantAttrs {
		if request.Attributes[i] != want {
			t.Fatalf("attribute[%d] = %q, want %q", i, request.Attributes[i], want)
		}
	}
}

func TestLdapDefaultsAndUserMapping(t *testing.T) {
	ldapService := &LdapService{}
	cfg := &config.Ldap{BaseDn: "dc=example,dc=com"}
	entry := ldap.NewEntry("uid=alice,ou=people,dc=example,dc=com", map[string][]string{
		"uid":       {"alice"},
		"mail":      {"alice@example.com"},
		"givenName": {"Alice"},
		"sn":        {"Example"},
		"memberOf":  {"cn=users,dc=example,dc=com"},
	})

	user := ldapService.userResultToLdapUser(cfg, entry)

	if ldapService.baseDnUser(cfg) != "dc=example,dc=com" {
		t.Fatalf("baseDnUser did not fall back to global base DN")
	}
	if user.Dn != entry.DN || user.Username != "alice" || user.Email != "alice@example.com" {
		t.Fatalf("unexpected user mapping: %#v", user)
	}
	if user.Name() != "Alice Example" {
		t.Fatalf("Name = %q", user.Name())
	}
	if !user.Enabled {
		t.Fatalf("user should be enabled when enable attr/value are not configured")
	}
}

func TestLdapUserAccountControlEnabledFlag(t *testing.T) {
	ldapService := &LdapService{}
	cfg := &config.Ldap{User: config.LdapUser{EnableAttr: "userAccountControl", EnableAttrValue: "unused-for-ad"}}

	enabledUser := &LdapUser{EnableAttrValue: "512"}
	if !ldapService.isUserEnabled(cfg, enabledUser) || !enabledUser.Enabled {
		t.Fatalf("userAccountControl 512 should be enabled")
	}

	disabledUser := &LdapUser{EnableAttrValue: "514"}
	if ldapService.isUserEnabled(cfg, disabledUser) || disabledUser.Enabled {
		t.Fatalf("userAccountControl 514 should be disabled")
	}
}

func TestLdapDirectMemberOfGroupChecks(t *testing.T) {
	ldapService := &LdapService{}
	cfg := &config.Ldap{User: config.LdapUser{
		AdminGroup: "cn=admins,dc=example,dc=com",
		AllowGroup: "cn=users,dc=example,dc=com",
	}}
	user := &LdapUser{Dn: "uid=alice,ou=people,dc=example,dc=com", MemberOf: []string{
		"CN=ADMINS,DC=EXAMPLE,DC=COM",
		"cn=users,dc=example,dc=com",
	}}

	if !ldapService.isUserAdmin(cfg, user) {
		t.Fatalf("direct memberOf admin group should be case-insensitive")
	}
	if !ldapService.isUserInGroup(cfg, user, cfg.User.AllowGroup) {
		t.Fatalf("direct memberOf allow group should pass")
	}
	if ldapService.isUserInGroup(cfg, user, "cn=other,dc=example,dc=com") {
		t.Fatalf("unlisted group should not pass")
	}
}

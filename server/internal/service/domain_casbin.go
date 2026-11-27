package service

import (
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	casbinpersist "github.com/casbin/casbin/v2/persist"
)

// Policy subjects are user UUIDs; policy roles are synthetic global names.
// Domain roles include the domain UUID, so identical role names cannot leak
// permissions across domains.
const domainCasbinModel = `
[request_definition]
r = sub, dom, act

[policy_definition]
p = sub, dom, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && (p.dom == "*" || p.dom == r.dom) && r.act == p.act
`

func domainCasbinModelText() string {
	return strings.TrimSpace(domainCasbinModel)
}

func newDomainEnforcer(adapter casbinpersist.Adapter) (*casbin.SyncedEnforcer, error) {
	parsedModel, err := casbinmodel.NewModelFromString(domainCasbinModelText())
	if err != nil {
		return nil, fmt.Errorf("load domain casbin model: %w", err)
	}
	enforcer, err := casbin.NewSyncedEnforcer(parsedModel, adapter)
	if err != nil {
		return nil, fmt.Errorf("init domain casbin enforcer: %w", err)
	}
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("load domain casbin policy: %w", err)
	}
	return enforcer, nil
}

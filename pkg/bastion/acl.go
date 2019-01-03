package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"sort"

	"moul.io/sshportal/pkg/dbmodels"
)

type byWeight []*dbmodels.ACL

func (a byWeight) Len() int           { return len(a) }
func (a byWeight) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byWeight) Less(i, j int) bool { return a[i].Weight < a[j].Weight }

func checkACLs(user dbmodels.User, host dbmodels.Host) (string, error) {
	// shared ACLs between user and host
	aclMap := map[uint]*dbmodels.ACL{}
	for _, userGroup := range user.Groups {
		for _, userGroupACL := range userGroup.ACLs {
			for _, hostGroup := range host.Groups {
				for _, hostGroupACL := range hostGroup.ACLs {
					if userGroupACL.ID == hostGroupACL.ID {
						aclMap[userGroupACL.ID] = userGroupACL
					}
				}
			}
		}
	}
	// FIXME: add ACLs that match host pattern

	// deny by default if no shared ACL
	if len(aclMap) == 0 {
		return string(dbmodels.ACLActionDeny), nil // default action
	}

	// transform map to slice and sort it
	acls := make([]*dbmodels.ACL, 0, len(aclMap))
	for _, acl := range aclMap {
		acls = append(acls, acl)
	}
	sort.Sort(byWeight(acls))

	return acls[0].Action, nil
}

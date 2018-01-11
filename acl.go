package main

import "sort"

type ByWeight []*ACL

func (a ByWeight) Len() int           { return len(a) }
func (a ByWeight) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByWeight) Less(i, j int) bool { return a[i].Weight < a[j].Weight }

func CheckACLs(user User, host Host) (string, error) {
	// shared ACLs between user and host
	aclMap := map[uint]*ACL{}
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
		return ACLActionDeny, nil // default action
	}

	// transform map to slice and sort it
	acls := make([]*ACL, 0, len(aclMap))
	for _, acl := range aclMap {
		acls = append(acls, acl)
	}
	sort.Sort(ByWeight(acls))

	return acls[0].Action, nil
}

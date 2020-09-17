package bastion

import (
	"encoding/json"
	"log"
	"os/exec"
	"sort"
	"strings"
	"time"

	"moul.io/sshportal/pkg/dbmodels"
)

type byWeight []*dbmodels.ACL

func (a byWeight) Len() int           { return len(a) }
func (a byWeight) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byWeight) Less(i, j int) bool { return a[i].Weight < a[j].Weight }

func checkACLs(user dbmodels.User, host dbmodels.Host, aclCheckCmd string) string {
	currentTime := time.Now()

	// shared ACLs between user and host
	aclMap := map[uint]*dbmodels.ACL{}
	for _, userGroup := range user.Groups {
		for _, userGroupACL := range userGroup.ACLs {
			for _, hostGroup := range host.Groups {
				for _, hostGroupACL := range hostGroup.ACLs {
					if userGroupACL.ID == hostGroupACL.ID {
						if (userGroupACL.Inception == nil || currentTime.After(*userGroupACL.Inception)) &&
							(userGroupACL.Expiration == nil || currentTime.Before(*userGroupACL.Expiration)) {
							aclMap[userGroupACL.ID] = userGroupACL
						}
					}
				}
			}
		}
	}
	// FIXME: add ACLs that match host pattern

	// deny by default if no shared ACL
	if len(aclMap) == 0 {
		return checkACLsHook(aclCheckCmd, string(dbmodels.ACLActionDeny), user, host) // default action
	}

	// transform map to slice and sort it
	acls := make([]*dbmodels.ACL, 0, len(aclMap))
	for _, acl := range aclMap {
		acls = append(acls, acl)
	}
	sort.Sort(byWeight(acls))

	return checkACLsHook(aclCheckCmd, acls[0].Action, user, host)
}

// checkACLsHook executes external command to check ACL and passes following parameters:
// $1 - SSH Portal action (`allow` or `deny`)
// $2 - User as JSON string
// $3 - Host as JSON string
// External program has to return `allow` or `deny` in stdout.
func checkACLsHook(aclCheckCmd string, action string, user dbmodels.User, host dbmodels.Host) string {
	if aclCheckCmd != "" {
		jsonUser, err := json.Marshal(user)
		if err != nil {
			log.Printf("Error: %v", err)
			return action
		}

		jsonHost, err := json.Marshal(host)
		if err != nil {
			log.Printf("Error: %v", err)
			return action
		}

		args := []string{
			action,
			string(jsonUser),
			string(jsonHost),
		}

		out, err := exec.Command(aclCheckCmd, args...).CombinedOutput()
		if err != nil {
			log.Printf("Error: %v", err)
			return action
		}

		outStr := strings.TrimSuffix(string(out), "\n")

		switch outStr {
		case string(dbmodels.ACLActionAllow):
			return string(dbmodels.ACLActionAllow)
		case string(dbmodels.ACLActionDeny):
			return string(dbmodels.ACLActionDeny)
		default:
			log.Printf("Error: acl-check-cmd wrong output '%s'\n", outStr)
			return action
		}
	}
	return action
}

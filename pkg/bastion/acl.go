package bastion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"time"

	"moul.io/sshportal/pkg/dbmodels"
)

// ACLHookTimeout is timeout for external ACL hook execution
const ACLHookTimeout = 2 * time.Second

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

	// if no shared ACL then execute ACLs hook if it exists and return its result
	if len(aclMap) == 0 {
		action, err := checkACLsHook(aclCheckCmd, string(dbmodels.ACLActionDeny), user, host)
		if err != nil {
			log.Println(err)
		}
		return action
	}

	// transform map to slice and sort it
	acls := make([]*dbmodels.ACL, 0, len(aclMap))
	for _, acl := range aclMap {
		acls = append(acls, acl)
	}
	sort.Sort(byWeight(acls))

	action, err := checkACLsHook(aclCheckCmd, acls[0].Action, user, host)
	if err != nil {
		log.Println(err)
	}

	return action
}

// checkACLsHook executes external command to check ACL and passes following parameters:
// $1 - SSH Portal `action` (`allow` or `deny`)
// $2 - User as JSON string
// $3 - Host as JSON string
// External program has to return `allow` or `deny` in stdout.
// In case of any error function returns `action`.
func checkACLsHook(aclCheckCmd string, action string, user dbmodels.User, host dbmodels.Host) (string, error) {
	if aclCheckCmd == "" {
		return action, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), ACLHookTimeout)
	defer cancel()

	jsonUser, err := json.Marshal(user)
	if err != nil {
		return action, err
	}

	jsonHost, err := json.Marshal(host)
	if err != nil {
		return action, err
	}

	args := []string{
		action,
		string(jsonUser),
		string(jsonHost),
	}

	cmd := exec.CommandContext(ctx, aclCheckCmd, args...)
	out, err := cmd.Output()
	if err != nil {
		return action, err
	}

	if ctx.Err() == context.DeadlineExceeded {
		return action, fmt.Errorf("external ACL hook command timed out")
	}

	outStr := strings.TrimSuffix(string(out), "\n")

	switch outStr {
	case string(dbmodels.ACLActionAllow):
		return string(dbmodels.ACLActionAllow), nil
	case string(dbmodels.ACLActionDeny):
		return string(dbmodels.ACLActionDeny), nil
	default:
		return action, fmt.Errorf("acl-check-cmd wrong output '%s'", outStr)
	}
}

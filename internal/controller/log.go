package controller

import "fmt"

func principalLogFields(principal *accountPrincipal) string {
	if principal == nil {
		return "email='<none>' account_id='<none>' organization_id='<none>'"
	}
	return fmt.Sprintf(
		"email='%s' account_id='%s' organization_id='%s'",
		principal.Email,
		principal.AccountID,
		principal.OrganizationID,
	)
}

func adminLogFields() string {
	return "admin='true'"
}

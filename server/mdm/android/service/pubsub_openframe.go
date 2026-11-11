package service

func openframeAndroidEnrollTeam(secretTeamID, priorTeamID *uint, priorFound, shared bool) *uint {
	if priorFound && !shared {
		return priorTeamID
	}
	return secretTeamID
}

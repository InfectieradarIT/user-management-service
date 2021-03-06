package timer_event

import (
	"log"
	"time"
)

func (s *UserManagementTimerService) CleanUpUnverifiedUsers() {
	log.Println("Starting clean up job for unverified users:")
	instances, err := s.globalDBService.GetAllInstances()
	if err != nil {
		log.Printf("unexpected error: %s", err.Error())
	}
	deleteUnverifiedUsersAfter := s.CleanUpTimeThreshold
	for _, instance := range instances {
		count, err := s.userDBService.DeleteUnverfiedUsers(instance.InstanceID, time.Now().Unix()-deleteUnverifiedUsersAfter)
		if err != nil {
			log.Printf("unexpected error: %s", err.Error())
			continue
		}
		log.Printf("%s: removed %d unverified accounts", instance.InstanceID, count)
	}
}

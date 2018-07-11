package shell

import (
	"os"
	"os/user"
	"strconv"
	"syscall"

	"fmt"

	"gitlab.com/gitlab-org/gitlab-runner/common"
)

// manages creation of directories for the builds and cache
// directories, and the permissions on them
func (s *executor) PrepareSetUIDDirectories(options common.ExecutorPrepareOptions, mapping func(string) string) error {
	variables := options.Build.GetAllVariables()

	// If the directory should go in the user's homedir
	// as specified by the config.toml, we need to ensure
	// that the directories are created with appropriate
	// permissions inside the user's homedir
	if options.Build.Runner.SetUIDDataDir == "$HOME" {
		rootDir := s.UserHomeDir + "/.gitlab-runner/"
		expandedBuildDir := rootDir + "/builds"
		expandedCacheDir := rootDir + "/cache"

		rootDirValidErr := s.CreateDirectory(rootDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)
		buildDirValidErr := s.CreateDirectory(expandedBuildDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)
		cacheDirValidErr := s.CreateDirectory(expandedCacheDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)

		if buildDirValidErr != nil || cacheDirValidErr != nil || rootDirValidErr != nil {
			return fmt.Errorf("Could not create top level data directories with permissions: %s. Failed with error: %s; %s; %s", "0750", buildDirValidErr, cacheDirValidErr, rootDirValidErr)
		}

		s.DefaultBuildsDir = expandedBuildDir
		s.DefaultCacheDir = expandedCacheDir

		// If the directory is specified in config.toml, the
		// builds and cache need to go in that directory with
		// correct permissions.
	} else if options.Build.Runner.SetUIDDataDir != "" {
		if _, err := os.Stat(options.Build.Runner.SetUIDDataDir); os.IsNotExist(err) {
			return fmt.Errorf("The data dir specified in config must exist with the correct ownership before builds can be run")
		}
		groupDir := options.Build.Runner.SetUIDDataDir + variables.Get("CI_PROJECT_NAMESPACE")
		// check to see if the directory defined above already exists.
		if _, err := os.Stat(groupDir); os.IsNotExist(err) {
			// path to data directory does not exist, so we should create it with
			// root:root - 701.
			createGroupDirErr := s.CreateDirectory(groupDir, 0, 0, 0701)
			if createGroupDirErr != nil {
				return fmt.Errorf("could not create top level data directories with permissions: %s. Failed with error: %s", "0701", createGroupDirErr)
			}
			// because the directory did not exist, we can continue down the
			// into the next layer of the desired directory structure.
			projectDir := groupDir + "/" + variables.Get("CI_PROJECT_NAME")
			createProjectDirErr := s.CreateDirectory(projectDir, 0, 0, 0701)
			if createProjectDirErr != nil {
				return fmt.Errorf("could not create data directory %s with permissions: %s. Failed with error: %s", projectDir, "0701", createProjectDirErr)
			}
			// split the directory into builds and cache
			buildDir := projectDir + "/" + "builds"
			cacheDir := projectDir + "/" + "cache"
			createBuildDirErr := s.CreateDirectory(buildDir, 0, 0, 0701)
			createCacheDirErr := s.CreateDirectory(cacheDir, 0, 0, 0701)
			if createCacheDirErr != nil || createBuildDirErr != nil {
				return fmt.Errorf("could not create builds and cache with permissions: %s. Failed with error: %s; %s", "0701", createBuildDirErr, createCacheDirErr)
			}
			// create the users directories under builds and cache
			usersBuildDir := buildDir + "/" + "users"
			usersCacheDir := cacheDir + "/" + "users"
			createUsersBuildDirErr := s.CreateDirectory(usersBuildDir, 0, 0, 0701)
			createUsersCacheDirErr := s.CreateDirectory(usersCacheDir, 0, 0, 0701)
			if createUsersBuildDirErr != nil || createUsersCacheDirErr != nil {
				return fmt.Errorf("could not create users builds and cache with permissions: %s. Failed with error: %s; %s", "0701", createUsersBuildDirErr, createUsersCacheDirErr)
			}
			// finally, create directories under users for the validated user.
			validatedUserBuildDir := usersBuildDir + "/" + s.ValidatedUser
			validatedUserCacheDir := usersCacheDir + "/" + s.ValidatedUser
			createValidatedUserBuildDirErr := s.CreateDirectory(validatedUserBuildDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)
			createValidatedUserCacheDirErr := s.CreateDirectory(validatedUserCacheDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)
			if createValidatedUserBuildDirErr != nil || createValidatedUserCacheDirErr != nil {
				return fmt.Errorf("could not create users builds and cache with permissions: %s. Failed with error: %s; %s", "0750", createUsersBuildDirErr, createUsersCacheDirErr)
			}
			// inform the runner that the build should go in this directory
			s.DefaultBuildsDir = validatedUserBuildDir
			s.DefaultCacheDir = validatedUserCacheDir
		} else {
			// the directory already, exists. get its group GID and mode, and
			// save them for use on all the directories underneath
			groupDirStats, osStatErr := os.Stat(groupDir)
			if osStatErr != nil {
				return osStatErr
			}

			// set the value of gid to match the GID of the parent directory
			// set the value of mode to match the mode of the parent directory
			gid := int(groupDirStats.Sys().(*syscall.Stat_t).Gid)
			var mode os.FileMode
			mode = 0750

			createGroupDirErr := s.CreateDirectory(groupDir, 0, gid, mode)
			if createGroupDirErr != nil {
				return fmt.Errorf("could not create top level data directories with permissions: %s. Failed with error: %s", mode, createGroupDirErr)
			}
			// because the directory did not exist, we can continue down the
			// into the next layer of the desired directory structure.
			projectDir := groupDir + "/" + variables.Get("CI_PROJECT_NAME")
			createProjectDirErr := s.CreateDirectory(projectDir, 0, gid, mode)
			if createProjectDirErr != nil {
				return fmt.Errorf("could not create data directory %s with permissions: %s. Failed with error: %s", projectDir, mode, createProjectDirErr)
			}
			// split the directory into builds and cache
			buildDir := projectDir + "/" + "builds"
			cacheDir := projectDir + "/" + "cache"
			createBuildDirErr := s.CreateDirectory(buildDir, 0, gid, mode)
			createCacheDirErr := s.CreateDirectory(cacheDir, 0, gid, mode)
			if createCacheDirErr != nil || createBuildDirErr != nil {
				return fmt.Errorf("could not create builds and cache with permissions: %s. Failed with error: %s; %s", mode, createBuildDirErr, createCacheDirErr)
			}
			// create the users directories under builds and cache
			usersBuildDir := buildDir + "/" + "users"
			usersCacheDir := cacheDir + "/" + "users"
			createUsersBuildDirErr := s.CreateDirectory(usersBuildDir, 0, gid, mode)
			createUsersCacheDirErr := s.CreateDirectory(usersCacheDir, 0, gid, mode)
			if createUsersBuildDirErr != nil || createUsersCacheDirErr != nil {
				return fmt.Errorf("could not create users builds and cache with permissions: %s. Failed with error: %s; %s", mode, createUsersBuildDirErr, createUsersCacheDirErr)
			}
			// finally, create directories under users for the validated user.
			validatedUserBuildDir := usersBuildDir + "/" + s.ValidatedUser
			validatedUserCacheDir := usersCacheDir + "/" + s.ValidatedUser
			createValidatedUserBuildDirErr := s.CreateDirectory(validatedUserBuildDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)
			createValidatedUserCacheDirErr := s.CreateDirectory(validatedUserCacheDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)
			if createValidatedUserBuildDirErr != nil || createValidatedUserCacheDirErr != nil {
				return fmt.Errorf("could not create users builds and cache with permissions: %s. Failed with error: %s; %s", "0750", createUsersBuildDirErr, createUsersCacheDirErr)
			}
			// placeholding
			s.DefaultBuildsDir = validatedUserBuildDir
			s.DefaultCacheDir = validatedUserCacheDir
		}
		// otherwise, the builds dir is the default and we just need
		// to create the users/ and cache/ dir underneath
	} else {
		expandedBuildDir := os.Expand(s.DefaultBuildsDir, mapping) + "/users/"
		expandedCacheDir := os.Expand(s.DefaultCacheDir, mapping) + "/users/"

		cacheDirValidErr := s.CreateDirectory(expandedBuildDir, 0, 0, 0751)
		buildDirValidErr := s.CreateDirectory(expandedCacheDir, 0, 0, 07E1)

		if cacheDirValidErr != nil || buildDirValidErr != nil {
			return fmt.Errorf("Could not create top level data directories with permissions: %s. Failed with error: %s; %s", "0751", cacheDirValidErr, buildDirValidErr)
		}

		userBuildDir := expandedBuildDir + s.ValidatedUser
		userCacheDir := expandedCacheDir + s.ValidatedUser

		userBuildDirValidErr := s.CreateDirectory(userBuildDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)
		userCacheDirValidErr := s.CreateDirectory(userCacheDir, s.ValidatedUserUID, s.ValidatedUserGID, 0750)

		if userBuildDirValidErr != nil || userCacheDirValidErr != nil {
			return fmt.Errorf("Could not create user data directories with permissions: %s. Failed with error: %s; %s", "0750", userBuildDirValidErr, userCacheDirValidErr)
		}

		s.DefaultBuildsDir = userBuildDir
		s.DefaultCacheDir = userCacheDir
	}
	return nil
}

// Ensure the specified user exits, and has permissions to do things.
// manage whitelist / blacklist functionality
func (s *executor) ValidateUser(options common.ExecutorPrepareOptions) error {
	variables := options.Build.GetAllVariables()
	gitlabUser := variables.Get("GITLAB_USER_LOGIN")

	// Set the UID/GID of the logged in user
	usr, usrErr := user.Lookup(gitlabUser)

	if usrErr != nil {
		return fmt.Errorf("golang was unable to perform a lookup on the logged in user %s: %s", gitlabUser, usrErr)
	}

	groupids, groupsErr := usr.GroupIds()
	var groups []string
	for _, gid := range groupids {
		groupptr, _ := user.LookupGroupId(gid)
		group := *groupptr
		groups = append(groups, group.Name)
	}

	if groupsErr != nil {
		return fmt.Errorf("golang was unable to get the list of groups that the logged in user, %s, is a member of. Are you sure the GitLab user exists on the CI system? %s", gitlabUser, groupsErr)
	}
	uid, _ := strconv.Atoi(usr.Uid)
	gid, _ := strconv.Atoi(usr.Gid)

	setuidUserWhitelist := options.Build.Runner.SetUIDUserWhitelist
	setuidUserBlacklist := options.Build.Runner.SetUIDUserBlacklist
	setuidGroupWhitelist := options.Build.Runner.SetUIDGroupWhitelist
	setuidGroupBlacklist := options.Build.Runner.SetUIDGroupBlacklist

	// is this user in a group that is also in the group whitelist or blacklist?
	var inGroupWhitelist bool
	var inGroupBlacklist bool

	// If the user is in groups that are on the groups blacklist, we need to inform
	// them that ALL of those groups which are blacklisted are blacklisted.
	var sharedBlacklistedGroups []string

	// If the user is in the user whitelist, we always run the job
	if contains(setuidUserWhitelist, gitlabUser) {
		s.ValidatedUserUID = uid
		s.ValidatedUserGID = gid
		s.ValidatedUser = gitlabUser
		s.UserHomeDir = usr.HomeDir
		return nil

		// If the user is not in the whitelist, check if the user is in the user
		// blacklist. If it is, we will deny always
	} else if contains(setuidUserBlacklist, gitlabUser) {
		return fmt.Errorf("the logged in user, %s, is not in the user whitelist and is in the user blacklist", gitlabUser)

		// If the user is not in either user list, check the group blacklist.
		// If the user is in the group blacklist, we deny
	} else if inGroupBlacklist, sharedBlacklistedGroups = CheckGroupBlacklist(setuidGroupBlacklist, groups); inGroupBlacklist {
		return fmt.Errorf("the logged in user, %s, is not on the user whitelist and is a member of the following groups that are on the groups blacklist, and is not allowed to run CI jobs: %s", gitlabUser, sharedBlacklistedGroups)

		// If the user is on no blacklists and is in any group on the groups
		// whitelist, they are allowed to run CI jobs.
	} else if inGroupWhitelist = CheckGroupWhitelist(setuidGroupWhitelist, groups); inGroupWhitelist {
		s.ValidatedUserUID = uid
		s.ValidatedUserGID = gid
		s.ValidatedUser = gitlabUser
		s.UserHomeDir = usr.HomeDir
		return nil

		// If there isnt a group whitelist at all, and we haven't failed yet, then
		// the user is allowed to run jobs.
	} else if len(setuidGroupWhitelist) == 0 {
		s.ValidatedUserUID = uid
		s.ValidatedUserGID = gid
		s.ValidatedUser = gitlabUser
		s.UserHomeDir = usr.HomeDir
		return nil

		// If the user is not in ANY lists AND there is a groups whitelist
		// defined, that user is not allowed to run jobs.
	} else if len(setuidGroupWhitelist) > 0 && !inGroupWhitelist {
		return fmt.Errorf("a group whitelist exists, but the user %s is not a member of any groups that are on that whitelist, and is not allowed to run CI jobs", gitlabUser)

		// finally, we have the case where something really wonky has happened. This
		// shouldn't be reachable, but it's good code practice
	} else {
		return fmt.Errorf("could not validate that user %s is allowed to run CI jobs. Please check that your whitelists and blacklists are properly formed according to the documentation", gitlabUser)
	}
}

// CheckGroupWhitelist checks if the user is in any group that is a member of the groups whitelist.
func CheckGroupWhitelist(groupWhitelist, userGroups []string) bool {
	// the group whitelist is empty. Stop.
	if !(len(groupWhitelist) > 0) {
		return false
	}
	for _, userGroup := range userGroups {
		if contains(groupWhitelist, userGroup) {
			return true
		}
	}
	return false
}

// CheckGroupBlacklist checks if the user is any group that is a member of the groups blacklist,
// return all shared groups.
func CheckGroupBlacklist(groupBlacklist, userGroups []string) (bool, []string) {
	var sharedGroups []string
	// the group blacklist is empty. Stop.
	if len(groupBlacklist) == 0 {
		return false, sharedGroups
	}
	for _, userGroup := range userGroups {
		if contains(groupBlacklist, userGroup) {
			sharedGroups = append(sharedGroups, userGroup)
		}
	}
	if len(sharedGroups) > 0 {
		return true, sharedGroups
	} else {
		return false, sharedGroups
	}
}

// check if an item exists in a list of items, return true if it does.
func contains(list []string, item string) bool {
	if len(list) == 0 {
		// the list is emptys
		return false
	}
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}

// given a string path, owernship, and mode -> create the directory with the given
// owner, group, and mode. If it already exists, do nothing
func (s *executor) CreateDirectory(dir string, owner, group int, mode os.FileMode) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// path/to/file does not exist
		buildErr := os.MkdirAll(dir, mode)

		if buildErr != nil {
			return buildErr
		}

		ownershipErr := s.EnsureOwnership(dir, owner, group)
		if ownershipErr != nil {
			return ownershipErr
		}
		return nil
	} else {
		// path to file exists already
		return nil
	}
}

func (s *executor) EnsureOwnership(dir string, uid, gid int) error {
	chownErr := os.Chown(dir, uid, gid)
	if chownErr != nil {
		return chownErr
	}

	return nil
}

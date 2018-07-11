# SetUID

## Overview

The SetUID extension to the GitLab Runner was developed on behalf of the US 
Department of Energy by OnyxPoint, INC. The work has been open-sourced in 
an effort to both stay true to the open-source roots of GitLab and to ensure
that US Govt. resources are leveraged to the benefit of the greater community.

The goal of the SetUID extension is to ensure that, given an environment with a
set of users each with varying levels of access, each user is allowed to run
CI jobs on the underlying CI systems and those CI jobs will not exceed the level
of access granted to the controlling user. 

This is accomplished by assuming a one-to-one relationship between user accounts
on the GitLab front-end and user accounts on the underlying linux-based CI 
systems. The CI job forks a process under the controlling user's system account,
which ensures that commands run during the CI job will fail if they impact
files or systems that the user does not have access to. 

## SetUID Runner Mechanics

Suppose there exists a user Alice, who has an account both on the GitLab instance 
and the underlying system upon which the GitLab Runner is configured to perform
CI jobs. Alice is a contributing member of a repository that leverages a shared
Runner to perform CI jobs. When the user Alice starts a CI job on the GitLab 
front-end, the following steps occur:

- The GitLab front-end indicates to all active runners that are configured to 
run CI jobs for Alice's project that a new job is pending
- A GitLab Runner on the underlying CI system pings the GitLab instance to check
for pending jobs, and finds that Alice's job is ready to be run
- The GitLab Runner receives the specifics of the CI job from the GitLab instance,
including the user who initiated the job (in this case Alice)
- The GitLab Runner, as root, configures the directory stucture that the job 
will run in on the CI system, as defined in the `configuration` section below
- The GitLab Runner creates a new process as the local user Alice, which 
clones the repo into the directory created previously and runs the CI job
defined in .gitlab-ci.yml
- If the job defined by .gitlab-ci.yml attempts to access system resources that
Alice's user does not have access to, the job will fail and indicate a lack of 
permissions

The above steps depend on the following assumptions: 
- The `alice` user exists both on the GitLab system and on the underlying linux-
based CI system. If `alice` does not exist on the CI system, the job will fail
upon attempting to fork a process to the non-existant `alice` user. It is 
expected that most of the time some underlying user access management system,
such as LDAP, will be responsible for managing users both on the CI system and 
the GitLab system
- A properly configured job is defined by `.gitlab-ci.yaml`
- A runner with the ability to run as a privileged user is configured to pick 
up CI jobs on Alice's project
- If a non-default builds directory is specified in `config.toml`, it must 
be exist on the underlying system and be properly configued. See `configuration`
below for more information

## Configuration

- The SetUID functionality adds a number of configuration options to the 
config.toml file. 

- `setuid` is a boolean that enables the SetUID functionality if set to true. 
- `setuid_data_dir` is a string that sets the directory on the CI system where 
CI jobs will be run. 

   - If this option is not set, builds will go into the default
     directory for CI jobs and a `users` directory will be created for each user's CI
     jobs. (`<default_builds_dir>/builds/users/<username>/<job>`)
   - If this option is set to the literal string `$HOME`, builds will go into the
     home directory of the user that the job is running as.
     (`</home/<username>/.gitlab-ci/<job>`)
   - If this option is set to anything else, the runner will check to see if the 
     speficied directory exists. If it does not, the runner will fail. If it does,
     builds will be placed in 
     `<specified_dir>/<group>/<project>/builds/users/<username>/<build>`
     Further, it is possible to configure the ownership of this directory structure
     by manually setting ownership of the <group> portion of the directory prior 
     to running jobs. 
   - If `<specified_dir>/<group>` does not exist, it will be created and owned by 
     `root:root` and mode 0701, with each underlying directory configured the same until
     the `<specified_dir>/<group>/<project>/builds/users/<username>` directory, which
     will be owned by `<username>:<username>`. This ensures that the user that 
     owns a CI job will be able to reach their job data, but not the jobs of other users.
   - If `<specified_dir>/<group>` does exist, all underlying directories will be owned
     by the group that owns `<specified_dir>/<group>`. This ensures that it is possible
     to configure user groups such that all users in a given group can access the 
     CI jobs of other users in that group.

- `setuid_user_whitelist` is a list of string usernames that will always be allowed
  to run CI jobs. This is the highest precedence list. 
- `setuid_user_blacklist` is a list of string usernames that will not be permitted 
  to run CI jobs. If a username is on both the whitelist and the blacklist, the user
  will be permitted to run jobs. 
- `setuid_groups_blacklist` is a list of string groupnames that will not be permitted
  to run CI jobs. If a user attempts to run a job and that user is a member of ANY
  of the groups in the groups blacklist, their job will fail with an error that
  indicates which groups they are a part of that are on the blacklist. 
  - User whitelist and blacklist take precedence over the groups whitelist and 
    blacklist. This enables sites to allow or disallow entire groups while making
    exceptions for individual users. 
  - The groups blacklist is higher precedence than the groups whitelist. This is
    slightly irregular, but due to the many-to-many relationship of groups that 
    a user can be assigned to and groups that may be on the blacklist, it is 
    safer to deny access to a user that is on both the groups whitelist and the
    groups blacklist than it is to permit access.
- `setuid_groups_whitelist` is a list of string group names that will be permitted 
  to run CI jobs. If a user is a member of any group that is on this list, and is 
  also neither in a group that is on the blacklist nor on the user blacklist, they 
  will be permitted to run CI jobs. 
  - If this key is not defined, and a user does not fall into any other whitelist
    or blacklist, then that user will be allowed to run jobs. If no whitelists 
    or blacklists are defined, then all users will be allowed to run jobs. 
  - If a user falls into both this list and a blacklist, they will not be allowed
    to run jobs. 
  - If a groups whitelist is defined but a user does not belong to a group on the
    whitelist, that user will not be allowed to run CI jobs. Again, defining a 
    groups whitelist implies that a user that is NOT on that whitelist should NOT
    be permitted to run CI jobs. If there is no groups whitelist defined, ALL users
    will be permitted run CI jobs, barring the effects of other whitelists / blacklists. 

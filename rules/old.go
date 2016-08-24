package rules

import (
	"time"

	"github.com/Masterminds/glide/cache"
	"github.com/Masterminds/glide/cfg"
	"github.com/Masterminds/glide/msg"
	"github.com/Masterminds/vcs"
)

func howOld(name string, dep *cfg.Dependency, lock *cfg.Lock, repo vcs.Repo) {

	refs, err := getAllVcsRefs(repo)
	if err != nil {
		msg.Die("Unable to get refs: %s", err)
	}

	srs := getSemVers(refs)
	if len(srs) == 0 {
		// Get latest commit on default branch or branch in use
		// Start with finding the branch, if there is one.
		ref := dep.Reference
		if dep.Reference == "" {
			key, err := cache.Key(repo.Remote())
			if err != nil {
				msg.Die("Unable to generate cache key: %s", err)
			}
			d, err := cache.RepoData(key)
			if err != nil {
				msg.Die("Unable to get cached repo data: %s", err)
			}
			ref = d.DefaultBranch
		}
		err = repo.UpdateVersion(ref)
		if err != nil {
			msg.Die("Error setting repo version on %s: %s", dep.Name, err)
		}

		curr, err := repo.Current()
		if err != nil {
			msg.Die("Error getting current version on %s: %s", dep.Name, err)
		}

		if curr == lock.Version {
			ci, err := repo.CommitInfo(lock.Version)
			if err == nil {
				msg.Puts("%s Using the latest revision on the selected or default branch (from: %s)", markD, ci.Date.Format(time.RFC3339))
			} else {
				msg.Puts("%s Using the latest revision on the selected or default branch", markD)
			}
			return
		}

		tci, err := repo.CommitInfo(curr)
		if err != nil {
			msg.Die("Unable to get commit info for %s: %s", dep.Name, err)
		}

		// Get info for current commit
		cci, err := repo.CommitInfo(lock.Version)
		if err != nil {
			msg.Die("Unable to get commit info for %s: %s", dep.Name, err)
		}

		// Display based on date difference
		dur := tci.Date.Sub(cci.Date)
		days := dur.Hours() / 24
		if days < 91 {
			msg.Puts("%s Using revision within three month from the tip of the branch (%.0f days)", markD, days)
		} else if days < 182 {
			msg.Puts("%s Using revision between three and six months from the tip of the branch (%.0f days)", markD, days)
		} else {
			msg.Puts("%s Using revision over six months behind the tip of the branch (%.0f days)", markX, days)
		}
	}
}

func isBranch(branch string, repo vcs.Repo) (bool, error) {
	branches, err := repo.Branches()
	if err != nil {
		return false, err
	}
	for _, b := range branches {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}

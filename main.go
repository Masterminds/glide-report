package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/glide-report/rules"
	"github.com/Masterminds/glide/action"
	"github.com/Masterminds/glide/cache"
	"github.com/Masterminds/glide/cfg"
	"github.com/Masterminds/glide/dependency"
	"github.com/Masterminds/glide/msg"
	gpath "github.com/Masterminds/glide/path"
	"github.com/Masterminds/glide/repo"
	"github.com/Masterminds/glide/util"
	"github.com/Masterminds/vcs"
)

var noColor bool

func init() {
	flag.BoolVar(&noColor, "no-color", false, "Disable color in report")
}

func main() {

	flag.Parse()
	if noColor {
		msg.Default.NoColor = true
		rules.Setup(noColor)
	}

	cache.Setup()
	err := cache.SystemLock()
	if err != nil {
		msg.Die(err.Error())
	}
	defer func() {
		cache.SystemUnlock()
	}()

	// Provide warnings
	msg.Warn("Disclaimer, this report is to help highlight things to consider. It is")
	msg.Warn("alpha software and the rules are still under consideration.")

	// Read the glide.yaml file to know range information
	msg.Info("Reading glide.yaml file to understand configured versions and ranges")
	conf := getConfFile()

	// Get the lockfile. This has the revisions we're using.
	msg.Info("Reading glide.lock file to understand pinned revisions")
	lock := getLockFile(conf)

	// Scan the code to determine direct vs transitive dependencies.
	roots, testRoots := getLocalDeps()

	// Fetch dependnecies to cache
	fetchMissingDeps(lock)

	marked := make(map[string]bool)
	testMarked := make(map[string]bool)

	msg.Puts("Report on %s\n", conf.Name)
	msg.Puts("------------------------------------------------------------------------------")
	msg.Puts("Direct Imports")
	msg.Puts("------------------------------------------------------------------------------\n")

	process(roots, conf, lock, marked, false)

	if len(conf.DevImports) > 0 {
		msg.Puts("\n------------------------------------------------------------------------------")
		msg.Puts("Direct Test Imports")
		msg.Puts("------------------------------------------------------------------------------\n")

		process(testRoots, conf, lock, testMarked, true)
	}

	deps := findTrans(roots, lock)
	if len(deps) > 0 {
		msg.Puts("------------------------------------------------------------------------------")
		msg.Puts("Transitive Imports")
		msg.Puts("------------------------------------------------------------------------------\n")

		process(deps, conf, lock, marked, false)
	}

}

func getConfFile() *cfg.Config {
	yamlpath, err := gpath.Glide()
	if err != nil {
		msg.Info("glide.yaml not found, attempting to generate while importing from other managers")
		msg.Default.Quiet = true
		action.Create(".", false, true)
		msg.Default.Quiet = false
		yamlpath, err = gpath.Glide()
		if err != nil {
			msg.ExitCode(2)
			msg.Die("Failed to find %s file in directory tree: %s", gpath.GlideFile, err)
		}
	}

	yml, err := ioutil.ReadFile(yamlpath)
	if err != nil {
		msg.Info("glide.yaml not found, attempting to generate while importing from other managers")
		msg.Default.Quiet = true
		action.Create(".", false, true)
		msg.Default.Quiet = false
		yml, err = ioutil.ReadFile(yamlpath)
		if err != nil {
			msg.ExitCode(2)
			msg.Die("Failed to load %s: %s", yamlpath, err)
		}
	}

	conf, err := cfg.ConfigFromYaml(yml)
	if err != nil {
		msg.ExitCode(3)
		msg.Die("Failed to parse %s: %s", yamlpath, err)
	}

	return conf
}

func getLockFile(conf *cfg.Config) *cfg.Lockfile {
	if !gpath.HasLock(".") {
		msg.Info("glide.lock not found, attempting to generate. This may take a moment...")
		msg.Default.Quiet = true
		action.EnsureGopath()

		installer := repo.NewInstaller()
		installer.Home = gpath.Home()
		installer.ResolveTest = true

		// Try to check out the initial dependencies.
		if err := installer.Checkout(conf); err != nil {
			msg.Die("Failed to do initial checkout of config: %s", err)
		}

		// Set the versions for the initial dependencies so that resolved dependencies
		// are rooted in the correct version of the base.
		if err := repo.SetReference(conf, installer.ResolveTest); err != nil {
			msg.Die("Failed to set initial config references: %s", err)
		}

		confcopy := conf.Clone()

		err := installer.Update(confcopy)
		if err != nil {
			msg.Die("Could not update packages: %s", err)
		}

		// Set references. There may be no remaining references to set since the
		// installer set them as it went to make sure it parsed the right imports
		// from the right version of the package.
		if err := repo.SetReference(confcopy, installer.ResolveTest); err != nil {
			msg.Die("Failed to set references: %s (Skip to cleanup)", err)
		}

		hash, err := conf.Hash()
		if err != nil {
			msg.Die("Failed to generate config hash. Unable to generate lock file.")
		}
		lock, err := cfg.NewLockfile(confcopy.Imports, confcopy.DevImports, hash)
		if err != nil {
			msg.Die("Failed to generate lock file: %s", err)
		}

		if err := lock.WriteFile(filepath.Join(".", gpath.LockFile)); err != nil {
			msg.Die("Could not write lock file to %s: %s", ".", err)
		}

		msg.Default.Quiet = false

		if !gpath.HasLock(".") {
			msg.Die("glide.lock file missing. Please generate first")
		}
	}

	lock, err := cfg.ReadLockFile(filepath.Join(".", gpath.LockFile))
	if err != nil {
		msg.Die("Could not load lockfile.")
	}

	return lock
}

func getLocalDeps() ([]string, []string) {
	basedir, err := filepath.Abs(".")
	if err != nil {
		msg.Die("Could not read directory: %s", err)
	}

	r, err := dependency.NewResolver(basedir)
	if err != nil {
		msg.Die("Could not create a resolver: %s", err)
	}
	h := &dependency.DefaultMissingPackageHandler{Missing: []string{}, Gopath: []string{}, Prefix: "vendor"}
	r.Handler = h

	pkgs, testPkgs, err := r.ResolveLocal(false)
	if err != nil {
		msg.Die("Error listing dependencies: %s", err)
	}

	// redure to just the roots
	var roots []string
	var roottests []string
	var found bool

	vd := filepath.Join(basedir, "vendor") + string(filepath.Separator)
	for _, p := range pkgs {
		found = false
		pp := strings.TrimPrefix(p, vd)
		ppp, _ := util.NormalizeName(pp)
		for _, v := range roots {
			if v == ppp {
				found = true
			}
		}
		if !found {
			roots = append(roots, ppp)
		}
	}

	for _, p := range testPkgs {
		found = false
		pp := strings.TrimPrefix(p, vd)
		ppp, _ := util.NormalizeName(pp)
		for _, v := range roottests {
			if v == ppp {
				found = true
			}
		}
		if !found {
			roots = append(roottests, ppp)
		}
	}

	sort.Strings(roots)
	sort.Strings(roottests)

	return roots, roottests
}

func fetchMissingDeps(lock *cfg.Lockfile) {

	msg.Info("Fetching dependency data, this may take a moment...")
	concurrentWorkers := 20
	done := make(chan struct{}, concurrentWorkers)
	in := make(chan *cfg.Lock, concurrentWorkers)
	var wg sync.WaitGroup

	for ii := 0; ii < concurrentWorkers; ii++ {
		go func(ch <-chan *cfg.Lock) {
			for {
				select {
				case ll := <-ch:
					dep := cfg.DependencyFromLock(ll)
					key, err := cache.Key(dep.Remote())
					p := filepath.Join(cache.Location(), "src", key)
					repo, err := dep.GetRepo(p)
					if err != nil {
						msg.Die("Unable to get repo for %s", dep.Name)
					}
					cache.Lock(key)

					if _, err = os.Stat(p); os.IsNotExist(err) {
						repo.Get()
						branch := findCurrentBranch(repo)
						c := cache.RepoInfo{DefaultBranch: branch}
						err = cache.SaveRepoData(key, c)
						if err != nil {
							msg.Die("Error saving cache repo details: %s", err)
						}
					} else {
						repo.Update()
					}
					cache.Unlock(key)
					wg.Done()
				case <-done:
					return
				}
			}
		}(in)
	}

	for _, dep := range lock.Imports {
		wg.Add(1)
		in <- dep
	}

	for _, dep := range lock.DevImports {
		wg.Add(1)
		in <- dep
	}

	wg.Wait()

	// Close goroutines setting the version
	for ii := 0; ii < concurrentWorkers; ii++ {
		done <- struct{}{}
	}
}

func findCurrentBranch(repo vcs.Repo) string {
	msg.Debug("Attempting to find current branch for %s", repo.Remote())
	// Svn and Bzr don't have default branches.
	if repo.Vcs() == vcs.Svn || repo.Vcs() == vcs.Bzr {
		return ""
	}

	if repo.Vcs() == vcs.Git || repo.Vcs() == vcs.Hg {
		ver, err := repo.Current()
		if err != nil {
			msg.Debug("Unable to find current branch for %s, error: %s", repo.Remote(), err)
			return ""
		}
		return ver
	}

	return ""
}

func process(roots []string, conf *cfg.Config, lock *cfg.Lockfile, marked map[string]bool, t bool) {
	rs := rules.Rules()

	var dep *cfg.Dependency
	var l *cfg.Lock
	for _, v := range roots {
		msg.Puts("Analysis of %s:", v)
		if _, f := marked[v]; f {
			continue
		}

		marked[v] = true

		l = nil
		for _, ll := range lock.Imports {
			if ll.Name == v {
				l = ll
			}
		}

		if l == nil && t {
			for _, ll := range lock.DevImports {
				if ll.Name == v {
					l = ll
				}
			}
		}

		if l == nil {
			msg.Die("Unable to find expected lock for %s", v)
		}

		dep = conf.Imports.Get(v)
		if dep == nil && t {
			dep = conf.DevImports.Get(v)
		}
		if dep == nil {
			dep = cfg.DependencyFromLock(l)
		}

		key, err := cache.Key(dep.Remote())
		if err != nil {
			msg.Die("Unable to create cache key: %s", err)
		}

		loc := filepath.Join(cache.Location(), "src", key)
		repo, err := dep.GetRepo(loc)
		if err != nil {
			msg.Die("Unable to get repo: %s", err)
		}

		for _, r := range rs {
			r(v, dep, l, repo)
		}

		msg.Puts("") // To put in a new line
	}
}

func findTrans(roots []string, lock *cfg.Lockfile) []string {
	var nr []string
	var found bool
	for _, v := range lock.Imports {
		found = false
		for _, vv := range roots {
			if vv == v.Name {
				found = true
				break
			}
		}
		if !found {
			nr = append(nr, v.Name)
		}
	}

	return nr
}

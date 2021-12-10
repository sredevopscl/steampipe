package version_map

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/turbot/steampipe/version"

	"github.com/turbot/steampipe/steampipeconfig/modconfig"

	filehelpers "github.com/turbot/go-kit/files"
	"github.com/turbot/steampipe/utils"

	"github.com/Masterminds/semver"
	"github.com/turbot/go-kit/helpers"
	"github.com/turbot/steampipe/constants"
)

// WorkspaceLock is a map of ModVersionMaps items keyed by the parent mod whose dependencies are installed
type WorkspaceLock struct {
	workspacePath   string
	InstallCache    DependencyVersionMap
	MissingVersions DependencyVersionMap

	modsPath      string
	installedMods VersionListMap
}

func LoadWorkspaceLock(workspacePath string) (*WorkspaceLock, error) {
	var installCache = make(DependencyVersionMap)
	lockPath := constants.WorkspaceLockPath(workspacePath)
	if helpers.FileExists(lockPath) {

		fileContent, err := os.ReadFile(lockPath)
		if err != nil {
			log.Printf("[TRACE] error reading %s: %s\n", lockPath, err.Error())
			return nil, err
		}
		err = json.Unmarshal(fileContent, &installCache)
		if err != nil {
			log.Printf("[TRACE] failed to unmarshal %s: %s\n", lockPath, err.Error())
			return nil, nil
		}
	}
	res := &WorkspaceLock{
		workspacePath:   workspacePath,
		modsPath:        constants.WorkspaceModPath(workspacePath),
		InstallCache:    installCache,
		MissingVersions: make(DependencyVersionMap),
	}

	if err := res.getInstalledMods(); err != nil {
		return nil, err
	}

	// populate the MissingVersions
	// (this removes missing items from the install cache)
	res.setMissing()

	return res, nil
}

// populate MissingVersions and UnreferencedVersions
func (l *WorkspaceLock) validate() error {

	return nil
}

// getInstalledMods returns a map installed mods, and the versions installed for each
func (l *WorkspaceLock) getInstalledMods() error {
	// recursively search for all the mod.sp files under the .steampipe/mods folder, then build the mod name from the file path
	modFiles, err := filehelpers.ListFiles(l.modsPath, &filehelpers.ListOptions{
		Flags:   filehelpers.FilesRecursive,
		Include: []string{"**/mod.sp"},
	})
	if err != nil {
		return err
	}

	// create result map - a list of version for each mod
	installedMods := make(VersionListMap, len(modFiles))
	// collect errors
	var errors []error

	for _, modfilePath := range modFiles {
		modName, version, err := l.parseModPath(modfilePath)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		// add this mod version to the map
		installedMods.Add(modName, version)
	}

	if len(errors) > 0 {
		return utils.CombineErrors(errors...)
	}
	l.installedMods = installedMods
	return nil
}

// GetUnreferencedMods returns a map of all installed mods which are not in the lock file
func (l *WorkspaceLock) GetUnreferencedMods() VersionListMap {
	var unreferencedVersions = make(VersionListMap)
	for name, versions := range l.installedMods {
		for _, version := range versions {
			if !l.ContainsModVersion(name, version) {
				unreferencedVersions.Add(name, version)
			}
		}
	}
	return unreferencedVersions
}

// identify mods which are in tInstallCache but not installed
// move them from InstallCache into MissingVersions
func (l *WorkspaceLock) setMissing() {
	// create a map of full modname to bool to allow simple checking
	flatInstalled := l.installedMods.FlatMap()

	for parent, deps := range l.InstallCache {
		// deps is a map of dep name to resolved contraint list
		// flatten and iterate
		depsFlat := deps.FlatMap()
		for fullName, resolvedConstraint := range depsFlat {
			if !flatInstalled[fullName] {
				// get the mod name from the constraint (fullName includes the version)
				name := resolvedConstraint.Name
				// remove this item from the install cache and add into missing
				l.MissingVersions[parent].Add(name, resolvedConstraint)
				l.InstallCache[parent].Remove(name, resolvedConstraint)
			}
		}
	}
}

// extract the mod name and version from the modfile path
func (l *WorkspaceLock) parseModPath(modfilePath string) (modName string, modVersion *semver.Version, err error) {
	modFullName, err := filepath.Rel(l.modsPath, filepath.Dir(modfilePath))
	if err != nil {
		return
	}
	return modconfig.ParseModFullName(modFullName)
}

func (l *WorkspaceLock) Save() error {
	content, err := json.MarshalIndent(l.InstallCache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(constants.WorkspaceLockPath(l.workspacePath), content, 0644)
}

// Delete deletes the lock file
func (l *WorkspaceLock) Delete() error {
	return os.Remove(constants.WorkspaceLockPath(l.workspacePath))
}

// DeleteMods removes mods from the lock file then, if it is empty, deletes the file
func (l *WorkspaceLock) DeleteMods(mods VersionConstraintMap, parent *modconfig.Mod) error {

	for modName, constraint := range mods {
		parentDependencies := l.InstallCache[parent.Name()]
		if parentDependencies == nil {
			return nil
		}

		// look for this mod in the lock file entries for this parent
		lockedVersions := parentDependencies[modName]
		if len(lockedVersions) == 0 {
			return nil
		}
		// NOTE: for now we only support a single version of each mod per parent
		// when we support aliases this restriction can be removed
		if len(lockedVersions) > 1 {
			return fmt.Errorf("parent %s has more than 1 version of dependency %s", parent.Name(), modName)
		}

	}
}

// GetMod looks for a lock file entry matching the given mod name
func (l *WorkspaceLock) GetMod(modName string, parent *modconfig.Mod) (*ResolvedVersionConstraint, error) {
	parentDependencies := l.InstallCache[parent.Name()]
	if parentDependencies == nil {
		return nil, nil
	}
	// look for this mod in the lock file entries for this parent
	lockedVersions := parentDependencies[modName]
	if len(lockedVersions) == 0 {
		return nil, nil
	}
	// NOTE: for now we only support a single version of each mod per parent
	// when we support aliases this restriction can be removed
	if len(lockedVersions) > 1 {
		return nil, fmt.Errorf("parent %s has more than 1 version of dependency %s", parent.Name(), modName)
	}
	return lockedVersions[0], nil
}

// GetLockedModVersion looks for a lock file entry matching the required constraint and returns nil if not found
func (l *WorkspaceLock) GetLockedModVersion(requiredModVersion *modconfig.ModVersionConstraint, parent *modconfig.Mod) (*ResolvedVersionConstraint, error) {
	lockedVersion, err := l.GetMod(requiredModVersion.Name, parent)
	if err != nil {
		return nil, err
	}
	if lockedVersion == nil {
		return nil, nil
	}

	// verify the locked version satisfies the version constraint
	if !requiredModVersion.Constraint.Check(lockedVersion.Version) {
		return nil, nil
	}

	return lockedVersion, nil
}

// EnsureLockedModVersion looks for a lock file entry matching the required mod name and returns an error if not found
func (l *WorkspaceLock) EnsureLockedModVersion(requiredModVersion *modconfig.ModVersionConstraint, parent *modconfig.Mod) (*ResolvedVersionConstraint, error) {
	lockedVersion, err := l.GetMod(requiredModVersion.Name, parent)
	if err != nil {
		return nil, err
	}
	if lockedVersion == nil {
		return nil, nil
	}

	// verify the locked version satisfies the version constraint
	if !requiredModVersion.Constraint.Check(lockedVersion.Version) {
		return nil, fmt.Errorf("failed to install dependencies for %s - locked version %s does not meet the constraint %s", parent.Name(), modconfig.ModVersionFullName(requiredModVersion.Name, lockedVersion.Version), requiredModVersion.Constraint.Original)
	}

	return lockedVersion, nil
}

// GetLockedModVersionConstraint looks for a lock file entry matching the required mod version and if found,
// returns it in the form of a ModVersionConstraint
func (l *WorkspaceLock) GetLockedModVersionConstraint(requiredModVersion *modconfig.ModVersionConstraint, parent *modconfig.Mod) (*modconfig.ModVersionConstraint, error) {
	lockedVersion, err := l.EnsureLockedModVersion(requiredModVersion, parent)
	if err != nil {
		return nil, err
	}
	// create a new ModVersionConstraint using the locked version
	lockedVersionFullName := modconfig.ModVersionFullName(requiredModVersion.Name, lockedVersion.Version)
	return modconfig.NewModVersionConstraint(lockedVersionFullName)
}

// ContainsModVersion returns whether the lockfile contains the given mod version
func (l *WorkspaceLock) ContainsModVersion(modName string, modVersion *semver.Version) bool {
	for _, modVersionMap := range l.InstallCache {
		for lockName, lockVersions := range modVersionMap {
			// we only support a single version at the moment but iterate anyway - we validate elsewhere
			for _, lockVersion := range lockVersions {
				// TODO consider handling of metadata
				if lockName == modName && lockVersion.Version.Equal(modVersion) && lockVersion.Version.Metadata() == modVersion.Metadata() {
					return true
				}
			}
		}
	}
	return false
}

func (l *WorkspaceLock) ContainsModConstraint(modName string, constraint *version.Constraints) bool {
	for _, modVersionMap := range l.InstallCache {
		for lockName, lockVersions := range modVersionMap {
			// we only support a single version at the moment but iterate anyway - we validate elsewhere
			for _, lockVersion := range lockVersions {
				if lockName == modName && lockVersion.Constraint == constraint.Original {
					return true
				}
			}
		}
	}
	return false
}

package version_map

import (
	"fmt"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/turbot/steampipe/steampipeconfig/modconfig"
)

// VersionListMap is a map keyed by dependency name storing a list of versions for each dependency
type VersionListMap map[string]semver.Collection

func (i VersionListMap) GetVersionSatisfyingRequirement(requiredVersion *modconfig.ModVersionConstraint) *semver.Version {
	// is this dependency installed
	versions, ok := i[requiredVersion.Name]
	if !ok {
		return nil
	}
	for _, v := range versions {
		if requiredVersion.Constraint.Check(v) {
			return v
		}
	}
	return nil
}

func (i VersionListMap) Add(name string, version *semver.Version) {
	versions := append(i[name], version)
	// reverse sort the versions
	sort.Sort(sort.Reverse(versions))
	i[name] = versions

}

// FlatMap converts the VersionListMap map into a bool map keyed by qualified dependency name
func (m VersionListMap) FlatMap() map[string]bool {
	var res = make(map[string]bool)
	for name, versions := range m {
		for _, version := range versions {
			key := fmt.Sprintf("%s@%s", name, version)
			res[key] = true
		}
	}
	return res
}
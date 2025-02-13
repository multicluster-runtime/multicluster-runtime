# Release Process

The multicluster-runtime Project is released on an as-needed basis, roughly 
following controller-runtime releases. The process is as follows:

1. An issue is proposing a new release with a changelog since the last release.
1. All [OWNERS](OWNERS) must LGTM this release.
1. Ensure that all go.mods point to the relevant Kubernetes and controller-runtime versions.
1. An OWNER runs `git tag -s $VERSION` and pushes the tag with `git push $VERSION`.
   `$VERSION` should be a valid semver version for the main directory, with the
   major and minor versions matching those of controller-runtime, and the patch
   release being multicluster-runtime specific. Use `-pre-1` and `-rc-1` suffixes
   to indicate pre-release versions, with `rc` meaning that this might be the final
   release if no blockers are found.
1. The upper commands must be repeated for all sub-go-modules `sub/go/module/$VERSION`.
1. The main tag is promoted to a release on GitHub with the changelog attached.
1. The release issue is closed.

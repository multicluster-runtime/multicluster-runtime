# Release Process

The multicluster-runtime Project is released on an as-needed basis, roughly 
following controller-runtime releases. The process is as follows:

1. An issue is proposing a new release with a changelog since the last release.
1. All [OWNERS](OWNERS) must LGTM this release. 
1. An OWNER runs `hack/release.sh v$MAJOR.$MINOR.$PATCH`.
1. Create a release branch with `git checkout -b release-$MAJOR.$MINOR` from `$VERSION`
   and push it to Github `git push origin release-$MAJOR.$MINOR`.
1. The main tag is promoted to a release on GitHub with the changelog attached.
1. The release issue is closed.

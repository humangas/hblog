# HOW TO RELEASE
How to release a new version.


## Release

```
# Update VERSION
$ vim version.go

# Update README(usage etc)
$ vim README.md

# Release
$ make release
```


## Delete

```
$ git push --delete origin <VERSION_TAG>
```

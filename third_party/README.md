# Third-party notices

Release archives include `third_party/licenses`, containing dependency licenses
and notices for the Linux, macOS, and Windows builds on amd64 and arm64, plus the
Go standard library license. `MODULES.txt` records the module versions and
`go/VERSION.txt` records the packaging toolchain.

The release workflow runs `bash scripts/licenses.sh` before building. To collect
the same files locally, run that command from a clean checkout with Go and Bash
installed. It downloads the pinned `go-licenses` v2.0.1 tool and resolves package
dependencies for all six targets. Generated output is ignored by Git; use a
fresh checkout after dependency removals to avoid retaining old notices.

The collector fails on unknown licenses. There is one reviewed exception:
`github.com/mattn/go-localereader` v0.0.1 declares MIT and names its author in
its README, but has no standalone license file. We include that original README
and the author's subsequently added [MIT license](https://github.com/mattn/go-localereader/blob/6bae6c923850ab5e0da9f36f69811ffe17064228/LICENSE),
copied verbatim to `go-localereader-LICENSE`. The collector requires this exact
dependency version; revisit the exception when upgrading it.

These notices describe third-party components; Plane CLI itself is licensed
under the root [LICENSE](../LICENSE). No Plane server code is distributed.

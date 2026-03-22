%global debug_package %{nil}
%bcond_without check
%define _binary_payload w9.xzdio

%global upstream_version 1.3.6
%global upstream_release 1
%global release_goflags %{?release_goflags}%{!?release_goflags:-mod=readonly -trimpath -buildvcs=false}
%global release_ldflags %{?release_ldflags}%{!?release_ldflags:-s -w -buildid=}

Name:           dns-update
Version:        %{?pkg_version}%{!?pkg_version:%{upstream_version}}
Release:        %{?pkg_release}%{!?pkg_release:%{upstream_release}}%{?dist}
Summary:        Keep DNS records aligned with egress IP addresses
License:        Apache-2.0
Source0:        %{name}-%{version}.tar.gz
BuildRequires:  golang
Requires(post): systemd
Requires(preun): systemd
Requires(postun): systemd

%description
dns-update probes the host's current egress IPv4 and IPv6 addresses and keeps
one DNS hostname's A and AAAA records aligned with that state. The package
installs hardened systemd service and timer units plus sample config and token
placeholder files under /etc/dns-update.

%prep
%autosetup

%build
export CGO_ENABLED=0
# Release package builds bias for smaller binaries while keeping the normal Go
# optimizer defaults intact. Dev and test builds do not use these flags.
export GOFLAGS="%{release_goflags}"
export GO_LDFLAGS="%{release_ldflags}"
case "%{_target_cpu}" in
  x86_64)
    export GOOS=linux GOARCH=amd64
    ;;
  aarch64)
    export GOOS=linux GOARCH=arm64
    ;;
  armv7hl)
    export GOOS=linux GOARCH=arm GOARM=7
    ;;
  *)
    echo "unsupported RPM target CPU: %{_target_cpu}" >&2
    exit 1
    ;;
esac
mkdir -p build/bin
go build -ldflags "$GO_LDFLAGS" -o build/bin/dns-update ./cmd/dns-update

%if %{with check}
%check
export DNS_UPDATE_SKIP_COVERAGE_TEST=1
export DNS_UPDATE_SKIP_MUTATION_TEST=1
go test ./...
%endif

%install
install -D -m 0755 build/bin/dns-update %{buildroot}%{_bindir}/dns-update
install -D -m 0644 deploy/systemd/dns-update.service %{buildroot}%{_unitdir}/dns-update.service
install -D -m 0644 deploy/systemd/dns-update.timer %{buildroot}%{_unitdir}/dns-update.timer
install -D -m 0644 deploy/systemd/dns-update.env %{buildroot}%{_sysconfdir}/dns-update/dns-update.env
install -D -m 0644 config.example.json %{buildroot}%{_sysconfdir}/dns-update/config.example.json
install -D -m 0600 cloudflare.token.example %{buildroot}%{_sysconfdir}/dns-update/cloudflare.token.example
install -D -m 0644 README.md %{buildroot}%{_docdir}/%{name}/README.md
install -D -m 0644 SECURITY.md %{buildroot}%{_docdir}/%{name}/SECURITY.md
install -D -m 0644 CONTRIBUTING.md %{buildroot}%{_docdir}/%{name}/CONTRIBUTING.md
install -D -m 0644 packaging/README.md %{buildroot}%{_docdir}/%{name}/packaging-README.md
install -D -m 0644 docs/dns-update.1 %{buildroot}%{_mandir}/man1/dns-update.1

%post
if [ "$1" -eq 1 ] ; then
    systemctl preset dns-update.service dns-update.timer >/dev/null 2>&1 || :
fi
systemctl daemon-reload >/dev/null 2>&1 || :

%preun
if [ "$1" -eq 0 ] ; then
    systemctl --no-reload disable --now dns-update.timer dns-update.service >/dev/null 2>&1 || :
fi

%postun
systemctl daemon-reload >/dev/null 2>&1 || :

%files
%defattr(-,root,root,-)
%license LICENSE
%doc %{_docdir}/%{name}/README.md
%doc %{_docdir}/%{name}/SECURITY.md
%doc %{_docdir}/%{name}/CONTRIBUTING.md
%doc %{_docdir}/%{name}/packaging-README.md
%dir %{_sysconfdir}/dns-update
%config(noreplace) %{_sysconfdir}/dns-update/dns-update.env
%config(noreplace) %{_sysconfdir}/dns-update/config.example.json
%config(noreplace) %{_sysconfdir}/dns-update/cloudflare.token.example
%{_bindir}/dns-update
%{_unitdir}/dns-update.service
%{_unitdir}/dns-update.timer
%{_mandir}/man1/dns-update.1*

%changelog
* Sun Mar 22 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.3.6-1
- Refresh the README, packaged docs, and dns-update(1) man page for current CLI behavior
- Clarify that introspection modes still load and validate the assembled configuration
- Restore the missing 1.3.4 release notes and refresh metadata for the 1.3.6 release

* Sun Mar 22 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.3.5-1
- Harden GitHub release publishing to stage assets on a draft release before publication
- Add an explicit GitHub-hosted rebuild path for an existing tag
- Roll the release line forward to 1.3.5
- Refresh release metadata for the 1.3.5 release

* Sun Mar 22 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.3.4-1
- Revalidate the repository against the current stable Go 1.26.1 toolchain
- Roll the release line forward to 1.3.4
- Refresh release metadata for the 1.3.4 release

* Sun Mar 22 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.3.3-1
- Fix GitHub CLI authentication in release attestation verification
- Reissue the 1.3.2 release line as 1.3.3 after the failed tag publish
- Refresh release metadata for the 1.3.3 release

* Sun Mar 22 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.3.2-1
- Rework GitHub Actions into fast PR, nightly, and release lanes
- Add release SBOM generation, artifact attestations, and reproducibility checks
- Refresh release metadata for the 1.3.2 release

* Sat Mar 21 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.3.1-1
- Fix the OSV scanner workflow YAML so the release pipeline passes its lint gate again
- Reissue the 1.3 release line as 1.3.1 after the failed 1.3.0 asset publish
- Refresh release metadata for the 1.3.1 release

* Sat Mar 21 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.3.0-1
- Add a CLI-only delete mode for removing managed A, AAAA, or both record families
- Keep single-family deletion targeted with dedicated provider delete planning and verification
- Refresh release metadata and operator documentation for the 1.3.0 release

* Sat Mar 21 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.2.0-1
- Add a CLI-only force-push flag that refreshes matching DNS records even when the observed egress IPs have not changed
- Refresh release, packaging, and deployment documentation for the 1.2.0 release

* Fri Mar 20 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.1.0-1
- Remove dead runtime plumbing in config loading, flag parsing, and effective config printing
- Add repository-wide CODEOWNERS ownership for omkhar
- Require code-owner review on main while allowing the single repository owner to merge self-authored pull requests

* Thu Mar 19 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.0.3-1
- Stop UPX-packing packaged binaries so the hardened systemd service remains compatible with MemoryDenyWriteExecute=yes
- Extend the multi-distro systemd integration test to install and exercise the actual built .deb and .rpm packages

* Thu Mar 19 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.0.2-1
- Fix the packaged systemd timer so future runs stay scheduled after an initial condition-check skip
- Extend the multi-distro systemd timer integration test to cover that skipped-first-activation regression

* Thu Mar 19 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.0.1-1
- Accept systemd-managed credential files with read-only 0440-style modes
- Fall back to /etc/dns-update/config.json for implicit CLI runs when a local config.json is absent
- Add multi-distro systemd timer integration coverage
- Refresh systemd and packaging documentation for the release

* Wed Mar 18 2026 dns-update Maintainers <opensource@dns-update.invalid> - 1.0-1
- Initial public release

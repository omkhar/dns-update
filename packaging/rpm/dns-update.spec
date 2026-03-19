%global debug_package %{nil}
%bcond_without check
%define _binary_payload w9.xzdio

%global upstream_version 1.0.2
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
upx --best build/bin/dns-update

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

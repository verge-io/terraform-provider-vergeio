// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func (a *API) applyCertificate(ctx context.Context, data *certificateModel, cert *vergeos.Certificate, row certificateDetail, prior certificateModel, configuredDomain string) error {
	if cert == nil {
		return fmt.Errorf("certificate response was empty")
	}
	id := int(cert.Key)
	publicRaw := strings.TrimSpace(row.Public)
	public := certificatePublic(publicRaw)
	pemName := domainFromPEM(public)
	domain, storedName, domainList := certificateNameFields(cert, row)
	chosen := chooseCertificateDomainName(cert.Type, domain, storedName, domainList, public)
	tflog.Debug(ctx, fmt.Sprintf(
		"certificate read id=%d type=%s domain=%q domainname=%q public=%q pem=%q chosen=%q",
		id, cert.Type, domain, storedName, summarizeCertificatePublic(publicRaw), pemName, chosen,
	))
	data.ID = idString(id)
	if cert.Type != "" {
		data.Type = types.StringValue(cert.Type)
	} else if knownString(prior.Type) {
		data.Type = prior.Type
	}
	data.Domain = stringFromAPI(domain, prior.Domain)
	data.DomainName = applyDomainName(prior.DomainName, chosen)
	data.DomainList = stringFromAPI(domainListForState(domainList, stringValue(prior.DomainList), configuredDomain), prior.DomainList)
	data.Description = stringFromAPI(cert.Description, prior.Description)
	data.PublicCertificate = keepPEM(public, prior.PublicCertificate)
	data.Chain = keepPEM(row.Chain, prior.Chain)
	data.ACMEServer = stringFromAPI(cert.ACMEServer, prior.ACMEServer)
	data.EABKid = keepConfiguredString(cert.EABKid, prior.EABKid)
	data.KeyType = stringFromAPI(cert.KeyType, prior.KeyType)
	data.RSAKeySize = stringFromAPI(cert.RSAKeySize, prior.RSAKeySize)
	data.Contact = intFromAPI(int(cert.Contact), prior.Contact)
	data.AgreeTOS = boolFromAPI(cert.AgreeTOS, prior.AgreeTOS)
	data.Valid = types.BoolValue(cert.Valid)
	data.AutoCreated = types.BoolValue(cert.AutoCreated)
	data.Expires = timestampFromAPI(cert.Expires)
	data.Created = timestampFromAPI(cert.Created)
	data.Modified = timestampFromAPI(cert.Modified)
	data.PrivateKeyWOVersion = prior.PrivateKeyWOVersion
	data.EABHMACKeyWOVersion = prior.EABHMACKeyWOVersion
	return nil
}

// certificateDetailFields is one read of the certificate row.
// Certificates.Get does not ask for domainname, and a public value of
// "self" is not a PEM. domainname is the name stored at create.
const certificateDetailFields = "domain,domainname,domainlist,public,chain"

type certificateDetail struct {
	Domain     string `json:"domain"`
	DomainName string `json:"domainname"`
	DomainList string `json:"domainlist"`
	Public     string `json:"public"`
	Chain      string `json:"chain"`
}

func (a *API) readCertificateDetail(ctx context.Context, id int) (certificateDetail, error) {
	var row certificateDetail
	if a == nil || a.http == nil {
		return row, fmt.Errorf("vergeio client is nil")
	}
	if id <= 0 {
		return row, fmt.Errorf("certificate id is empty")
	}
	err := a.getJSON(ctx, vergeio.ObjectPath(certificateCollection, strconv.Itoa(id)), &vergeio.Options{Fields: certificateDetailFields}, &row)
	return row, err
}

// certificatePublic drops the sentinel VergeOS uses when a generated
// certificate has no PEM yet. Parsing that text is not a domain lookup.
func certificatePublic(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "self") {
		return ""
	}
	return raw
}

func summarizeCertificatePublic(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "BEGIN CERTIFICATE") {
		return "pem"
	}
	return raw
}

func certificateWithID(cert *vergeos.Certificate, id int) *vergeos.Certificate {
	if cert == nil {
		if id <= 0 {
			return nil
		}
		cert = &vergeos.Certificate{}
	}
	if int(cert.Key) <= 0 && id > 0 {
		cert.Key = vergeos.FlexInt(id)
	}
	return cert
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if name := strings.TrimSpace(value); name != "" {
			return name
		}
	}
	return ""
}

// loadCertificate reads the certificate row. wait is set on create.
// A create keeps reading until domain, domainname, and domainlist are
// filled in, or until the timeout. A read does the same only while the
// row is still new. A read error is returned as-is. The timeout returns
// an error. An empty result is not stored as success.
// ensureCertificateOwned returns NotFound when the stored key now points at
// another certificate (created mismatch). Skip the check when created is
// unset (import, or state written before this check existed). A nil data
// is a create, which has no stored created time yet.
func ensureCertificateOwned(data *certificateModel, cert *vergeos.Certificate) error {
	if data == nil {
		return nil
	}
	if cert == nil {
		return fmt.Errorf("certificate read returned no row")
	}
	if !knownInt(data.Created) || data.Created.ValueInt64() == 0 {
		return nil
	}
	if cert.Created != data.Created.ValueInt64() {
		return &vergeos.NotFoundError{Resource: "Certificate", ID: cert.Key.Int()}
	}
	return nil
}

func (a *API) loadCertificate(ctx context.Context, id int, wait bool, want string, owned *certificateModel) (*vergeos.Certificate, certificateDetail, error) {
	timeout := a.certificateNameTimeout
	if timeout <= 0 {
		timeout = certificateNameTimeout
	}
	freshFor := a.certificateFreshFor
	if freshFor <= 0 {
		freshFor = certificateNameTimeout
	}
	interval := a.certificateNameInterval
	deadline := time.Now().Add(timeout)
	for {
		cert, row, err := a.fetchCertificate(ctx, id)
		if err != nil {
			return nil, certificateDetail{}, err
		}
		if err := ensureCertificateOwned(owned, cert); err != nil {
			return nil, certificateDetail{}, err
		}
		domain, domainName, domainList := certificateNameFields(cert, row)
		if certificateNamesReady(cert.Type, domain, domainName, domainList) {
			return cert, row, nil
		}
		if certificateNamesContradict(want, domain, domainName, domainList) {
			if wait {
				return nil, certificateDetail{}, certificateWrongNameError(id, want, domain, domainName, domainList)
			}
			return cert, row, nil
		}
		if !wait && !certificateRowFresh(cert.Created, time.Now(), freshFor) {
			return cert, row, nil
		}
		if !time.Now().Before(deadline) {
			return nil, certificateDetail{}, fmt.Errorf(
				"certificate %d names were not ready after %s: domain=%q domainname=%q domainlist=%q",
				id, timeout, domain, domainName, domainList,
			)
		}
		if err := sleepContext(ctx, interval); err != nil {
			return nil, certificateDetail{}, err
		}
	}
}

func (a *API) fetchCertificate(ctx context.Context, id int) (*vergeos.Certificate, certificateDetail, error) {
	got, err := a.sdk.Certificates.Get(ctx, id)
	if err != nil {
		return nil, certificateDetail{}, err
	}
	row, err := a.readCertificateDetail(ctx, id)
	if err != nil {
		return nil, certificateDetail{}, err
	}
	return certificateWithID(got, id), row, nil
}

func certificateNameFields(cert *vergeos.Certificate, row certificateDetail) (domain, domainName, domainList string) {
	if cert == nil {
		return strings.TrimSpace(row.Domain), strings.TrimSpace(row.DomainName), strings.TrimSpace(row.DomainList)
	}
	return firstNonEmpty(row.Domain, cert.Domain), strings.TrimSpace(row.DomainName), firstNonEmpty(row.DomainList, cert.DomainList)
}

// certificateNamesReady reports that VergeOS has filled the name columns.
// A letsencrypt certificate can be issued long after domainlist is stored,
// so an empty domainname is enough for that type.
func certificateNamesReady(certType, domain, domainName, domainList string) bool {
	if strings.TrimSpace(domain) == "" || strings.TrimSpace(domainList) == "" {
		return false
	}
	if certType == vergeos.CertificateTypeLetsEncrypt {
		return true
	}
	return strings.TrimSpace(domainName) != ""
}

// certificateNamesContradict reports a row that already has names, and
// none of them is the name Terraform asked for. An empty row is still
// filling in. A row that already contains want is still filling in when
// domain or domainname is blank.
func certificateNamesContradict(want, domain, domainName, domainList string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	if strings.TrimSpace(domain) == "" && strings.TrimSpace(domainName) == "" && strings.TrimSpace(domainList) == "" {
		return false
	}
	return !certificateIssuedFor(want, domain, domainName, domainList)
}

func certificateIssuedFor(want, domain, domainName, domainList string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	if strings.TrimSpace(domain) == want || strings.TrimSpace(domainName) == want {
		return true
	}
	for _, part := range strings.Split(domainList, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

func certificateWrongNameError(id int, want, domain, domainName, domainList string) error {
	return fmt.Errorf(
		"certificate %d was issued for domain=%q domainname=%q domainlist=%q, not %q",
		id, strings.TrimSpace(domain), strings.TrimSpace(domainName), strings.TrimSpace(domainList), strings.TrimSpace(want),
	)
}

func certificateRowFresh(created int64, now time.Time, window time.Duration) bool {
	if created <= 0 || window <= 0 {
		return false
	}
	age := now.Sub(time.Unix(created, 0))
	return age >= 0 && age <= window
}

// certificateCreateList is the domainlist value sent to VergeOS.
// domainname is read-only. The primary name is the first entry. A later
// copy of that name is dropped. An empty primary leaves list unchanged.
func certificateCreateList(domainName, domainList string) string {
	domainName = strings.TrimSpace(domainName)
	if domainName == "" {
		return domainList
	}
	names := []string{domainName}
	seen := map[string]struct{}{domainName: {}}
	for _, part := range strings.Split(domainList, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		names = append(names, part)
	}
	return strings.Join(names, ",")
}

// domainListForState keeps a configured list when VergeOS stored that
// list with domain_name added at the front. The plan then matches.
func domainListForState(api, configured, domainName string) string {
	api = strings.TrimSpace(api)
	if api == "" {
		return ""
	}
	configured = strings.TrimSpace(configured)
	if configured == "" || api == configured {
		if configured == "" {
			return api
		}
		return configured
	}
	if certificateCreateList(domainName, configured) == certificateCreateList(domainName, api) {
		return configured
	}
	return api
}

// reconcileDomainName keeps a configured domain_name when the certificate
// was issued for that name. A self_signed certificate that was issued for
// other names stores the issued name, so the next plan replaces it.
// Import has no prior name, so chosen is the name from this read.
func reconcileDomainName(certType string, prior types.String, chosen, domain, storedName, domainList string) types.String {
	kept := applyDomainName(prior, chosen)
	if certType != vergeos.CertificateTypeSelfSigned || !knownString(prior) {
		return kept
	}
	want := strings.TrimSpace(prior.ValueString())
	if want == "" || certificateIssuedFor(want, domain, storedName, domainList) {
		return kept
	}
	issued := strings.TrimSpace(firstNonEmpty(storedName, domain, firstDomain(domainList)))
	if issued == "" {
		return kept
	}
	return types.StringValue(issued)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// applyDomainName keeps a domain_name already in state. VergeOS later
// stores the issued certificate CN in domainname. A normal read keeps
// the name from create when that certificate was issued for the name.
// Import starts with only the certificate key, so chosen is the name
// from this read.
func applyDomainName(prior types.String, chosen string) types.String {
	if knownString(prior) {
		return prior
	}
	if strings.TrimSpace(chosen) == "" {
		return types.StringNull()
	}
	return types.StringValue(strings.TrimSpace(chosen))
}

// chooseCertificateDomainName picks the import name.
// A manual certificate keeps domain when VergeOS reports one.
// Any other type uses domainname when that field is present, and does
// not use the certificate subject in that case.
func chooseCertificateDomainName(certType, domain, storedName, domainList, publicPEM string) string {
	domain = strings.TrimSpace(domain)
	storedName = strings.TrimSpace(storedName)
	if certType == vergeos.CertificateTypeManual && domain != "" {
		return domain
	}
	if storedName != "" {
		return storedName
	}
	if domain != "" {
		return domain
	}
	if name := firstDomain(domainList); name != "" {
		return name
	}
	return domainFromPEM(certificatePublic(publicPEM))
}

func firstDomain(list string) string {
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			return part
		}
	}
	return ""
}

func domainFromPEM(pemText string) string {
	rest := []byte(pemText)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return ""
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		parsed, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if cn := strings.TrimSpace(parsed.Subject.CommonName); cn != "" {
			return cn
		}
		for _, name := range parsed.DNSNames {
			name = strings.TrimSpace(name)
			if name != "" {
				return name
			}
		}
	}
}

func boolPtr(v types.Bool) *bool {
	if !knownBool(v) {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func certificateUpdateEmpty(req *vergeos.CertificateUpdateRequest) bool {
	if req == nil {
		return true
	}
	return req.DomainList == nil &&
		req.Description == nil &&
		req.Public == nil &&
		req.Private == nil &&
		req.Chain == nil &&
		req.ACMEServer == nil &&
		req.EABKid == nil &&
		req.EABHMACKey == nil &&
		req.KeyType == nil &&
		req.RSAKeySize == nil &&
		req.Contact == nil &&
		req.AgreeTOS == nil &&
		req.Renew == nil
}

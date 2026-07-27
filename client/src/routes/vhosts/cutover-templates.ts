// Client message templates for the Cutover Assistant.
//
// SOURCE OF TRUTH: design-templates `docs/cloudflare-tenant-cutover.md` @ 1a5eb56.
// These are transcriptions of the canonical templates — the modal RENDERS them, it
// does not invent copy. If a template changes there, change it here. Plain text
// only: the output must paste into an email client with no markdown artefacts.

export type CutoverVars = {
    domain: string;
    name: string; // client name, typed by the operator
    oldIp: string; // resolved live, never typed
    txt: { name: string; value: string }[]; // Phase 1: pasted from the CF dashboard
    nameservers: string[]; // Cloudflare's assigned pair
};

// The paragraph appended automatically whenever MX records were found — the
// client's actual fear is their email breaking.
export const EMAIL_UNAFFECTED =
    "Your email is unaffected — the MX records that route your mail are separate\nand stay exactly as they are.";

const fallback = (v: string, placeholder: string) => (v.trim() ? v.trim() : placeholder);

// Method C — custom hostname, TXT validation. The ten-minute pause between the TXT
// records and the A-record flip is what prevents the certificate race; it must not
// be edited out.
export function methodCMessage(v: CutoverVars, hasMx: boolean): string {
    const domain = fallback(v.domain, "{{DOMAIN}}");
    const rows = [0, 1, 2].map((i) => {
        const t = v.txt[i];
        const n = fallback(t?.name ?? "", `{{TXT_${i + 1}_NAME}}`);
        const val = fallback(t?.value ?? "", `{{TXT_${i + 1}_VALUE}}`);
        return `  Type: TXT   Name: ${n}   Value: ${val}`;
    });

    let body = `Subject: DNS changes for ${domain}

Hi ${fallback(v.name, "{{NAME}}")},

We're moving ${domain} onto a global content network — the site will load
faster worldwide and gains protection against attacks. Nothing changes about
how you use it.

There are two parts, both at your domain registrar, in one sitting.

PART 1 — add these records. They only confirm you own the domain; nothing
about the site changes yet.

${rows.join("\n")}

Enter each Name exactly as written — your registrar adds the rest of the
domain automatically.

PART 2 — wait ten minutes, then make the switch.

The pause matters: during those minutes we issue the security certificate
for your site. Switching before it's ready would show visitors a browser
warning.

  1. EDIT the existing A record
       Name: @
       Change the value from ${fallback(v.oldIp, "{{OLD_IP}}")} to  104.21.69.82

  2. ADD a second A record
       Type: A   Name: @   Value: 172.67.206.132   TTL: 600 seconds

  3. LEAVE the TXT records above exactly as they are — they keep the
     certificate renewing automatically.

Please don't change anything else.

Two addresses rather than one is for redundancy: if one entry point is ever
unreachable, visitors are served by the other automatically.

The site stays online throughout and switches over within about ten minutes.`;

    if (hasMx) body += `\n\n${EMAIL_UNAFFECTED}`;
    return body;
}

// Method A — nameserver delegation. One client action, permanently. Stage 4 of the
// guarded workflow; unlocked only once the DNSSEC and zone-diff gates are green.
export function methodAMessage(v: CutoverVars, hasMx: boolean): string {
    const domain = fallback(v.domain, "{{DOMAIN}}");
    const ns = v.nameservers.filter((n) => n.trim());
    const nsLines = (ns.length ? ns : ["{{NS_1}}", "{{NS_2}}"]).map((n) => `  ${n.trim()}`).join("\n");

    let body = `Subject: One DNS change for ${domain}

Hi ${fallback(v.name, "{{NAME}}")},

We're moving ${domain} onto a global content network — the site will load
faster worldwide and gains protection against attacks. Nothing changes about
how you use it.

This is a one-time change at your domain registrar, and it's the last DNS
request we'll need to make of you.

Please change the domain's nameservers to these two:

${nsLines}

Your registrar will have a field called "Nameservers", "DNS servers" or
"Custom DNS". Replace what's there now with the two above.

We have already copied across every existing record for this domain —
your website, your email, and anything else it points to — so nothing is
lost in the move. The change takes up to 48 hours to spread worldwide, and
your site and email keep working normally throughout.

Please don't change anything else.`;

    if (hasMx) body += `\n\n${EMAIL_UNAFFECTED}`;
    return body;
}

// Method B — HTTP validation. SUPERSEDED and ranked last: the client still adds
// _cf-custom-hostname AND still changes the A record, so it saves them nothing
// while adding an SSH step for us. Kept only to finish an in-flight domain.
export function methodBMessage(v: CutoverVars, hasMx: boolean): string {
    const domain = fallback(v.domain, "{{DOMAIN}}");
    const t = v.txt[0];

    let body = `Subject: DNS changes for ${domain}

Hi ${fallback(v.name, "{{NAME}}")},

We're moving ${domain} onto a global content network — the site will load
faster worldwide and gains protection against attacks. Nothing changes about
how you use it.

There are two parts, both at your domain registrar, in one sitting.

PART 1 — add this record. It only confirms you own the domain; nothing
about the site changes yet.

  Type: TXT   Name: ${fallback(t?.name ?? "", "_cf-custom-hostname")}   Value: ${fallback(t?.value ?? "", "{{TXT_1_VALUE}}")}

Enter the Name exactly as written — your registrar adds the rest of the
domain automatically.

PART 2 — wait ten minutes, then make the switch.

The pause matters: during those minutes we issue the security certificate
for your site. Switching before it's ready would show visitors a browser
warning.

  1. EDIT the existing A record
       Name: @
       Change the value from ${fallback(v.oldIp, "{{OLD_IP}}")} to  104.21.69.82

  2. ADD a second A record
       Type: A   Name: @   Value: 172.67.206.132   TTL: 600 seconds

Please don't change anything else.

The site stays online throughout and switches over within about ten minutes.`;

    if (hasMx) body += `\n\n${EMAIL_UNAFFECTED}`;
    return body;
}

// The www warning — www.<domain> needs its own custom hostname or it breaks at
// cutover (Method C / B only; under Method A the whole zone moves).
export function wwwWarning(domain: string): string {
    return `www.${domain} has a live DNS record today. Under a custom-hostname cutover it needs its OWN custom hostname in Cloudflare, or it breaks when the apex switches.`;
}

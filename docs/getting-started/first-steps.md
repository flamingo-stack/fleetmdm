# First Steps

You have Fleet running and your first host enrolled. Here are the five most important things to explore and configure next.

---

## 1. Explore the Dashboard

Open [http://localhost:8080](http://localhost:8080) and get familiar with the main navigation:

| Section | What You'll Find |
|---|---|
| **Dashboard** | Host counts, OS distribution, MDM enrollment status, activity feed |
| **Hosts** | Full inventory table — filter by OS, label, policy compliance, or MDM status |
| **Software** | Installed software inventory, vulnerability counts, Fleet Maintained Apps catalog |
| **Policies** | Compliance policies with pass/fail host counts |
| **Queries** | Saved osquery queries; run them live or on a schedule |
| **Controls** | OS settings profiles, MDM configuration, scripts, and setup experience |
| **Settings** | Organization config, integrations, MDM certificates, users, and teams |

Take a few minutes to click through each section. The Dashboard gives you a health overview; **Hosts** is where you spend most of your operational time.

---

## 2. Create Your First Policy

Policies are SQL queries that return either "passing" (rows found) or "failing" (no rows). They are the foundation of Fleet's compliance engine.

**Example: Verify FileVault is enabled on macOS**

1. Go to **Policies** → **Add policy**
2. Choose **Create your own policy**
3. Enter the following SQL:

```sql
SELECT 1
FROM disk_encryption
WHERE user_uuid IS NOT NULL
  AND filevault_status = 'on';
```

4. Set the **Name** to `FileVault enabled` and the **Platform** to `macOS`
5. Click **Save policy**

Within a few minutes, hosts will begin reporting pass/fail results as their agents check in.

> **Tip:** Fleet ships with a built-in policy library. Click **Add policy → Choose from library** to browse pre-written CIS benchmark and security checks.

---

## 3. Run a Live Query

Live queries let you run ad-hoc osquery SQL against your fleet in real time.

1. Go to **Queries** → **New query**
2. Enter this SQL to list all running processes with open network connections:

```sql
SELECT p.name, p.pid, p.path, l.address, l.port
FROM processes p
JOIN listening_ports l ON p.pid = l.pid
WHERE l.address != ''
ORDER BY p.name;
```

3. Click **Run query**
4. Select **All hosts** (or filter by label/platform)
5. Click **Run** and watch results stream in

Results appear per-host as agents respond. You can export the full result set as CSV from the results view.

---

## 4. Add Software to Your Library

Fleet can deploy, update, and uninstall software across your fleet. To add your first package:

1. Go to **Software** → **Add software**
2. Choose one of:
   - **Fleet-maintained apps** — pre-packaged apps like Google Chrome, Zoom, Slack, VS Code
   - **Custom package** — upload a `.pkg`, `.msi`, `.deb`, or `.rpm` file
   - **App Store (VPP)** — add iOS/macOS apps from your Apple Business Manager account
3. Configure install/uninstall scripts as needed
4. Optionally link the software to a policy for automatic remediation

Once added, you can deploy the software to hosts manually or automatically when a policy fails.

---

## 5. Configure GitOps (Infrastructure as Code)

Fleet supports a full GitOps workflow where all configuration lives in YAML files in your source repository. This is the recommended approach for production deployments.

**Install the `fleetctl` CLI:**

```bash
go build -o fleetctl ./cmd/fleetctl/...
```

**Create a GitOps config file (`fleet.yml`):**

```yaml
controls:
  macos_settings:
    custom_settings: []

policies: []

queries: []

agent_options:
  config:
    options:
      distributed_interval: 30
      logger_tls_period: 10
```

**Apply the config:**

```bash
fleetctl gitops --config fleet.yml --fleet-url http://localhost:8080
```

From this point forward, all changes to your fleet's configuration are tracked in Git and applied via CI/CD.

---

## Common Initial Configuration

### Create Teams

Teams allow you to scope policies, queries, and software to subsets of your fleet:

1. Go to **Settings** → **Teams** → **Create team**
2. Name the team (e.g., `Engineering`, `Finance`)
3. Assign enrollment secrets, agent options, and policies per team

### Set Up Email (SMTP)

Configure SMTP to enable Fleet to send invitation emails, password resets, and failure alerts:

1. Go to **Settings** → **Organization settings** → **SMTP**
2. Enter your SMTP server details
3. Click **Save** and **Test** to verify delivery

### Review Enroll Secrets

Each team has unique enroll secrets that bind an agent to that team:

1. Go to **Settings** → **Enroll secrets** (for the global fleet)
2. Or go to **Settings** → **Teams** → select a team → **Enroll secrets**
3. Copy the secret to use when packaging the `fleetd` agent

---

## Where to Get Help

- **Community Slack:** [openmsp.ai](https://www.openmsp.ai/) — join the `#fleetmdm` channel
- **Repository:** [github.com/flamingo-stack/fleetmdm](https://github.com/flamingo-stack/fleetmdm)
- **Flamingo Platform:** [flamingo.run](https://flamingo.run)
- **OpenFrame docs:** [openframe.ai](https://openframe.ai)

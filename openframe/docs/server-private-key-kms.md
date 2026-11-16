# Server private key through Cloud KMS

Fleet encrypts its stored secrets in MySQL with the server private key: the APNs and SCEP keys in
`mdm_config_assets`, `ca_config_assets`, certificate authority credentials, ABM and VPP tokens,
secret variables, recovery-lock and managed local account passwords. Upstream reads that key from
`FLEET_SERVER_PRIVATE_KEY` or from AWS Secrets Manager. This fork can instead keep the key in MySQL,
wrapped with a Google Cloud KMS key, so the plaintext key exists only in the Fleet process memory.

## Configuration

| Env var | YAML key | Description |
|---------|----------|-------------|
| `FLEET_SERVER_PRIVATE_KEY_KMS_KEY` | `server.private_key_kms_key` | Full Cloud KMS key name, `projects/<project>/locations/<location>/keyRings/<ring>/cryptoKeys/<key>`, without a key version. |

It cannot be combined with `server.private_key` or `server.private_key_arn`; Fleet refuses to start
if it is. The KMS client uses Application Default Credentials (Workload Identity on GKE). The
Fleet service account needs `roles/cloudkms.cryptoKeyEncrypterDecrypter` on that key.

## Boot

Before the datastore is created, `fleet serve` opens a short MySQL connection and reads the single
row of `openframe_server_private_key` (fork migration `20261116000001`):

- **Row exists:** its `kms_key_name` must equal the configured key, then Cloud KMS decrypts
  `wrapped_key` and the result becomes `config.Server.PrivateKey`.
- **No row:** if any table above already holds rows, Fleet refuses to start, because those rows are
  encrypted with another key. Otherwise it generates 32 random bytes (base64, like
  `openssl rand -base64 32`), encrypts them with Cloud KMS, `INSERT IGNORE`s the row and reads it
  back. Servers starting together all end up with the row of the first insert.
- **No table:** Fleet refuses to start and asks for `fleet prepare db`.

Cloud KMS calls carry CRC32C checksums and a fixed additional authenticated data string, and are
retried for up to a minute; 400, 401, 403, 404 and 412 answers fail at once. The key is never logged.

## Operations

- Every Fleet cluster on one MySQL database must use the same KMS key.
- Rotating the KMS key (new primary version) is safe: the ciphertext names its version. Never
  disable or destroy a version that wrapped the stored key, or Fleet cannot start and the stored
  secrets are lost.
- A database restored into another environment needs access to the original KMS key.
- The `fleet` subcommands that read `server.private_key` directly (`mdm_apple.go`) do not use this
  path.
- An environment that already has data encrypted with `FLEET_SERVER_PRIVATE_KEY` cannot switch to
  KMS with this change: there is no import of an existing key.

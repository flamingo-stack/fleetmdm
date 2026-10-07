package com.fleetdm.agent.scep

import android.util.Log
import com.fleetdm.agent.GetCertificateTemplateResponse
import org.bouncycastle.asn1.DERPrintableString
import org.bouncycastle.asn1.pkcs.PKCSObjectIdentifiers
import org.bouncycastle.asn1.x500.X500Name
import org.bouncycastle.asn1.x509.Extension
import org.bouncycastle.asn1.x509.ExtensionsGenerator
import org.bouncycastle.cert.jcajce.JcaX509CertificateConverter
import org.bouncycastle.cert.jcajce.JcaX509v3CertificateBuilder
import org.bouncycastle.jce.provider.BouncyCastleProvider
import org.bouncycastle.operator.jcajce.JcaContentSignerBuilder
import org.bouncycastle.pkcs.jcajce.JcaPKCS10CertificationRequestBuilder
import org.jscep.client.Client
import org.jscep.client.verification.OptimisticCertificateVerifier
import java.math.BigInteger
import java.net.URL
import java.security.KeyPairGenerator
import java.security.Security
import java.security.cert.Certificate
import java.util.Date
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Implementation of ScepClient using jScep library and BouncyCastle cryptography.
 *
 * This implementation performs SCEP enrollment by:
 * 1. Generating an RSA key pair
 * 2. Creating a self-signed certificate for PKCS7 envelope signing
 * 3. Building a Certificate Signing Request (CSR) with challenge password
 * 4. Sending enrollment request to SCEP server
 * 5. Extracting and returning the issued certificate and private key
 */
class ScepClientImpl : ScepClient {

    companion object {
        // SCEP `message=` value (CA identifier) sent on GetCACaps and GetCACert. Per RFC 8894
        // this is OPTIONAL and only meaningful when the endpoint represents multiple CAs.
        // Sending null causes jScep to omit the parameter; the server returns its default CA.
        private val SCEP_PROFILE: String? = null
        private const val SELF_SIGNED_CERT_VALIDITY_DAYS = 100L
        private const val TAG = "ScepClientImpl"

        init {
            // Ensure BouncyCastle provider is loaded
            if (Security.getProvider(BouncyCastleProvider.PROVIDER_NAME) == null) {
                Security.addProvider(BouncyCastleProvider())
            }
        }
    }

    override suspend fun enroll(config: GetCertificateTemplateResponse, scepUrl: String): ScepResult = withContext(Dispatchers.IO) {
        try {
            // Step 1: Generate key pair
            val keyPair = generateKeyPair(config.keyLength)

            // Step 2: Parse subject name
            val entity = try {
                X500Name(config.subjectName)
            } catch (e: Exception) {
                throw ScepCsrException("Invalid X.500 subject name: ${config.subjectName}", e)
            }

            // Step 3: Create self-signed certificate for signing the PKCS7 envelope
            val selfSignedCert = createSelfSignedCertificate(
                entity,
                keyPair,
                config.signatureAlgorithm,
            )

            // Step 4: Create SCEP client
            val server = try {
                URL(scepUrl)
            } catch (e: Exception) {
                throw ScepNetworkException("Invalid SCEP URL: $scepUrl", e)
            }

            // NOTE: OptimisticCertificateVerifier accepts any server certificate presented
            // during enrollment without validation. This is a known weakness: if scepUrl or
            // DNS resolution is ever manipulated, the client could complete enrollment
            // against an attacker-controlled CA and leak the challenge password embedded in
            // the CSR. A proper fix requires pinning against a known certificate/fingerprint
            // supplied by the MDM server when available. Until that plumbing exists, we keep
            // OptimisticCertificateVerifier as a fallback but this should be revisited.
            val verifier = OptimisticCertificateVerifier()
            val client = Client(server, verifier)

            // Step 5: Build Certificate Signing Request (CSR)
            val challenge = config.scepChallenge
            if (challenge.isNullOrEmpty()) {
                throw ScepCsrException("SCEP challenge password is missing; refusing to enroll without it")
            }
            val csr = buildCsr(
                entity,
                keyPair,
                challenge,
                config.signatureAlgorithm,
                config.subjectAlternativeName,
            )

            // Step 6: Send enrollment request
            val response = try {
                client.enrol(selfSignedCert, keyPair.private, csr, SCEP_PROFILE)
            } catch (e: Exception) {
                throw ScepNetworkException("Failed to communicate with SCEP server", e)
            }

            // Step 7: Process response
            when {
                response.isSuccess -> {
                    val certificates = extractCertificates(response.certStore)

                    if (certificates.isEmpty()) {
                        throw ScepCertificateException("No certificates returned from SCEP server")
                    }

                    val leafCertificate = (certificates.first() as java.security.cert.X509Certificate)
                    // Extract certificate metadata from the leaf certificate
                    val notAfter = leafCertificate.notAfter
                    val notBefore = leafCertificate.notBefore
                    val serialNumber = leafCertificate.serialNumber

                    ScepResult(
                        privateKey = keyPair.private,
                        certificateChain = certificates,
                        notAfter = notAfter,
                        notBefore = notBefore,
                        serialNumber = serialNumber,
                    )
                }
                response.isPending -> {
                    throw ScepEnrollmentException(
                        "Enrollment is pending - requires CA administrator approval",
                    )
                }
                response.isFailure -> {
                    throw ScepEnrollmentException(
                        "Enrollment failed with SCEP failInfo: ${response.failInfo}",
                    )
                }
                else -> {
                    throw ScepEnrollmentException(
                        "Enrollment failed - unexpected SCEP response",
                    )
                }
            }
        } catch (e: ScepException) {
            Log.e(TAG, "SCEP enrollment failed: ${e.message}", e)
            throw e
        } catch (e: Exception) {
            Log.e(TAG, "Unexpected SCEP enrollment error: ${e.message}", e)
            throw ScepException("Unexpected SCEP enrollment error: ${e.message}", e)
        }
    }

    private fun generateKeyPair(keyLength: Int) = try {
        val keyGen = KeyPairGenerator.getInstance("RSA")
        keyGen.initialize(keyLength)
        keyGen.genKeyPair()
    } catch (e: Exception) {
        throw ScepKeyGenerationException("Failed to generate RSA key pair", e)
    }

    private fun createSelfSignedCertificate(entity: X500Name, keyPair: java.security.KeyPair, signatureAlgorithm: String) = try {
        val now = System.currentTimeMillis()
        val validityEnd = now + (1000L * 60 * 60 * 24 * SELF_SIGNED_CERT_VALIDITY_DAYS)

        val certBuilder = JcaX509v3CertificateBuilder(
            entity,
            BigInteger.valueOf(1),
            Date(now),
            Date(validityEnd),
            entity,
            keyPair.public,
        )

        val contentSigner = JcaContentSignerBuilder(signatureAlgorithm).build(keyPair.private)
        val certHolder = certBuilder.build(contentSigner)

        JcaX509CertificateConverter().getCertificate(certHolder)
    } catch (e: Exception) {
        throw ScepCertificateException("Failed to create self-signed certificate", e)
    }

    internal fun buildCsr(
        entity: X500Name,
        keyPair: java.security.KeyPair,
        challenge: String,
        signatureAlgorithm: String,
        subjectAlternativeName: String?,
    ) = try {
        val csrBuilder = JcaPKCS10CertificationRequestBuilder(entity, keyPair.public)

        // Add challenge password attribute
        val passwordAttr = DERPrintableString(challenge)
        csrBuilder.addAttribute(PKCSObjectIdentifiers.pkcs_9_at_challengePassword, passwordAttr)

        // Add Subject Alternative Name extension if a non-empty SAN string was provided.
        val generalNames = try {
            SubjectAlternativeNameParser.parse(subjectAlternativeName)
        } catch (e: IllegalArgumentException) {
            throw ScepCsrException("Invalid subject alternative name: ${e.message}", e)
        }
        if (generalNames != null) {
            val extensions = ExtensionsGenerator().apply {
                addExtension(Extension.subjectAlternativeName, false, generalNames)
            }.generate()
            csrBuilder.addAttribute(PKCSObjectIdentifiers.pkcs_9_at_extensionRequest, extensions)
        }

        val contentSigner = JcaContentSignerBuilder(signatureAlgorithm).build(keyPair.private)
        csrBuilder.build(contentSigner)
    } catch (e: ScepCsrException) {
        throw e
    } catch (e: Exception) {
        throw ScepCsrException("Failed to build Certificate Signing Request", e)
    }

    private fun extractCertificates(certStore: java.security.cert.CertStore): List<Certificate> = try {
        val certificates = certStore.getCertificates(null)
        certificates.toList()
    } catch (e: Exception) {
        throw ScepCertificateException("Failed to extract certificates from response", e)
    }
}

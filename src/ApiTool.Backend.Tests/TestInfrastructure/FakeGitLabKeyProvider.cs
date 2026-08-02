// Deterministic fake IGitLabKeyProvider for tests — no key material or file I/O.
using ApiTool.Backend.GitLab;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// An identity-passthrough <see cref="IGitLabKeyProvider"/> for integration tests.
/// Returns a fixed kid so tests can assert on the round-trip without provisioning a KEK file.
/// </summary>
public sealed class FakeGitLabKeyProvider : IGitLabKeyProvider
{
    /// <summary>Sentinel kid returned by this fake.</summary>
    public const string FakeKid = "fake-gitlab-provider-v1";

    /// <inheritdoc/>
    public Task<GitLabEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
    {
        // Identity: return plaintext as ciphertext (test only — never use in production).
        return Task.FromResult(new GitLabEncryptionResult((byte[])plaintext.Clone(), FakeKid));
    }

    /// <inheritdoc/>
    public Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
    {
        return Task.FromResult((byte[])ciphertext.Clone());
    }
}

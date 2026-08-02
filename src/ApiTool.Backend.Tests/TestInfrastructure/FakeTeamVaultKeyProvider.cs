// Deterministic fake ITeamVaultKeyProvider for tests — no key material or file I/O.
using ApiTool.Backend.VaultConfig.Keys;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// An identity-passthrough <see cref="ITeamVaultKeyProvider"/> for integration tests.
/// Returns a fixed kid so tests can assert on the round-trip without provisioning a KEK file.
/// </summary>
public sealed class FakeTeamVaultKeyProvider : ITeamVaultKeyProvider
{
    /// <summary>Sentinel kid returned by this fake.</summary>
    public const string FakeKid = "fake-team-vault-provider-v1";

    /// <inheritdoc/>
    public Task<TeamVaultEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
    {
        // Identity: return plaintext as ciphertext (test only — never use in production).
        return Task.FromResult(new TeamVaultEncryptionResult((byte[])plaintext.Clone(), FakeKid));
    }

    /// <inheritdoc/>
    public Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
    {
        return Task.FromResult((byte[])ciphertext.Clone());
    }
}

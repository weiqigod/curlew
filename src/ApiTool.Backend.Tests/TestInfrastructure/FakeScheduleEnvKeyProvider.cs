// Deterministic fake IScheduleEnvKeyProvider for tests — no key material or file I/O.
using ApiTool.Backend.Schedules.Keys;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// An identity-passthrough <see cref="IScheduleEnvKeyProvider"/> for integration tests.
/// Returns a fixed kid so tests can assert on the round-trip without provisioning a KEK file.
/// </summary>
public sealed class FakeScheduleEnvKeyProvider : IScheduleEnvKeyProvider
{
    /// <summary>Sentinel kid returned by this fake.</summary>
    public const string FakeKid = "fake-schedule-env-provider-v1";

    /// <inheritdoc/>
    public Task<ScheduleEnvEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
    {
        // Identity: return plaintext as ciphertext (test only — never use in production).
        return Task.FromResult(new ScheduleEnvEncryptionResult((byte[])plaintext.Clone(), FakeKid));
    }

    /// <inheritdoc/>
    public Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
    {
        return Task.FromResult((byte[])ciphertext.Clone());
    }
}

// Tests for FileScheduleEnvKeyProvider — round-trip, startup validation, error sanitisation.
// Refs M18-009 (v4-12).
using System.Security.Cryptography;
using ApiTool.Backend.Schedules.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Schedules.Keys;

/// <summary>
/// Tests for <see cref="FileScheduleEnvKeyProvider"/>:
/// round-trip env_vars encrypt/decrypt, missing KEK file, wrong-size KEK, kid mismatch,
/// tampered ciphertext without leaking key bytes.
/// </summary>
public sealed class FileScheduleEnvKeyProviderTests : IDisposable
{
    private readonly string _tmpDir;
    private readonly string _kekPath;

    public FileScheduleEnvKeyProviderTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_sekek_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
        _kekPath = Path.Combine(_tmpDir, "schedule-env-kek.bin");
        File.WriteAllBytes(_kekPath, RandomNumberGenerator.GetBytes(32));
        if (OperatingSystem.IsLinux() || OperatingSystem.IsMacOS())
            File.SetUnixFileMode(_kekPath, UnixFileMode.UserRead | UnixFileMode.UserWrite);
    }

    public void Dispose()
    {
        if (Directory.Exists(_tmpDir))
            Directory.Delete(_tmpDir, recursive: true);
    }

    private FileScheduleEnvKeyProvider Create() => new(Options.Create(new ScheduleEnvEncryptionOptions
    {
        KeyProvider = new ScheduleEnvEncryptionOptions.KeyProviderConfig
        {
            Mode = "file",
            File = new ScheduleEnvEncryptionOptions.FileConfig { KekPath = _kekPath },
        },
    }));

    [Fact]
    public async Task Encrypt_then_Decrypt_round_trips_env_vars()
    {
        var p = Create();
        var pt = System.Text.Encoding.UTF8.GetBytes("{\"API_KEY\":\"secret-value\",\"ENV\":\"prod\"}");
        var result = await p.EncryptAsync(pt);
        result.Kid.Should().Be(FileScheduleEnvKeyProvider.KidValue);
        var dec = await p.DecryptAsync(result.Ciphertext, result.Kid);
        dec.Should().Equal(pt);
    }

    [Fact]
    public async Task Encrypt_produces_different_ciphertext_each_call()
    {
        var p = Create();
        var pt = System.Text.Encoding.UTF8.GetBytes("{\"KEY\":\"same-value\"}");
        var r1 = await p.EncryptAsync(pt);
        var r2 = await p.EncryptAsync(pt);
        r1.Ciphertext.Should().NotEqual(r2.Ciphertext);
    }

    [Fact]
    public async Task Encrypt_throws_when_kek_missing()
    {
        var p = new FileScheduleEnvKeyProvider(Options.Create(new ScheduleEnvEncryptionOptions
        {
            KeyProvider = new ScheduleEnvEncryptionOptions.KeyProviderConfig
            {
                Mode = "file",
                File = new ScheduleEnvEncryptionOptions.FileConfig { KekPath = "/does/not/exist.bin" },
            },
        }));
        // KEK load is deferred to first encrypt/decrypt call.
        var act = () => p.EncryptAsync(new byte[] { 1, 2, 3 });
        await act.Should().ThrowAsync<InvalidOperationException>()
            .WithMessage("*does/not/exist.bin*");
    }

    [Fact]
    public async Task Encrypt_throws_when_kek_wrong_size()
    {
        var badPath = Path.Combine(_tmpDir, "bad.bin");
        File.WriteAllBytes(badPath, new byte[16]);
        var p = new FileScheduleEnvKeyProvider(Options.Create(new ScheduleEnvEncryptionOptions
        {
            KeyProvider = new ScheduleEnvEncryptionOptions.KeyProviderConfig
            {
                Mode = "file",
                File = new ScheduleEnvEncryptionOptions.FileConfig { KekPath = badPath },
            },
        }));
        // KEK load is deferred to first encrypt/decrypt call.
        var act = () => p.EncryptAsync(new byte[] { 1, 2, 3 });
        await act.Should().ThrowAsync<InvalidOperationException>()
            .WithMessage("*16 bytes*32 bytes*");
    }

    [Fact]
    public async Task Decrypt_with_wrong_kid_throws_ScheduleEnvDecryptException()
    {
        var p = Create();
        var (ciphertext, _) = await p.EncryptAsync(new byte[] { 1, 2, 3 });
        var act = () => p.DecryptAsync(ciphertext, "some-other-kid");
        var ex = (await act.Should().ThrowAsync<ScheduleEnvDecryptException>()).Which;
        ex.Message.Should().Be("schedule env_vars decryption failed (provider=file)");
    }

    [Fact]
    public async Task Decrypt_with_tampered_ciphertext_throws_without_leaking_key_bytes()
    {
        var p = Create();
        var (blob, kid) = await p.EncryptAsync(System.Text.Encoding.UTF8.GetBytes("{\"SECRET\":\"value\"}"));
        blob[^1] ^= 0xFF;  // flip a tag bit → GCM authentication failure
        var act = () => p.DecryptAsync(blob, kid);
        var ex = (await act.Should().ThrowAsync<ScheduleEnvDecryptException>()).Which;
        ex.Message.Should().NotContain("kek");
        ex.Message.Should().NotContain(_kekPath);
        ex.Message.Should().Be("schedule env_vars decryption failed (provider=file)");
    }
}

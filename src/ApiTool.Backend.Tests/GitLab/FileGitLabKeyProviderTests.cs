// Tests for FileGitLabKeyProvider — round-trip, startup validation, error sanitisation.
// Refs docs/SPECIFICATION.md:9196-9205 (IGitLabKeyProvider), M16-013.
using System.Security.Cryptography;
using ApiTool.Backend.GitLab;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.GitLab;

/// <summary>
/// Tests for <see cref="FileGitLabKeyProvider"/>:
/// round-trip PAT encrypt/decrypt, missing KEK file, wrong-size KEK, kid mismatch,
/// tampered ciphertext without leaking key bytes.
/// </summary>
public sealed class FileGitLabKeyProviderTests : IDisposable
{
    private readonly string _tmpDir;
    private readonly string _kekPath;

    public FileGitLabKeyProviderTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_glkek_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
        _kekPath = Path.Combine(_tmpDir, "gitlab-kek.bin");
        File.WriteAllBytes(_kekPath, RandomNumberGenerator.GetBytes(32));
        if (OperatingSystem.IsLinux() || OperatingSystem.IsMacOS())
            File.SetUnixFileMode(_kekPath, UnixFileMode.UserRead | UnixFileMode.UserWrite);
    }

    public void Dispose()
    {
        if (Directory.Exists(_tmpDir))
            Directory.Delete(_tmpDir, recursive: true);
    }

    private FileGitLabKeyProvider Create() => new(Options.Create(new GitLabOptions
    {
        KeyProvider = new GitLabOptions.KeyProviderConfig
        {
            Mode = "file",
            File = new GitLabOptions.FileConfig { KekPath = _kekPath },
        },
    }));

    [Fact]
    public async Task Encrypt_then_Decrypt_round_trips_a_PAT()
    {
        var p = Create();
        var pt = System.Text.Encoding.UTF8.GetBytes("glpat-abcdef0123456789ABCDEF");
        var result = await p.EncryptAsync(pt);
        result.Kid.Should().Be(FileGitLabKeyProvider.KidValue);
        var dec = await p.DecryptAsync(result.Ciphertext, result.Kid);
        dec.Should().Equal(pt);
    }

    [Fact]
    public async Task Encrypt_produces_different_ciphertext_each_call()
    {
        var p = Create();
        var pt = System.Text.Encoding.UTF8.GetBytes("glpat-same-input");
        var r1 = await p.EncryptAsync(pt);
        var r2 = await p.EncryptAsync(pt);
        r1.Ciphertext.Should().NotEqual(r2.Ciphertext);
    }

    [Fact]
    public void Constructor_throws_when_kek_missing()
    {
        var act = () => new FileGitLabKeyProvider(Options.Create(new GitLabOptions
        {
            KeyProvider = new GitLabOptions.KeyProviderConfig
            {
                Mode = "file",
                File = new GitLabOptions.FileConfig { KekPath = "/does/not/exist.bin" },
            },
        }));
        act.Should().Throw<InvalidOperationException>()
            .WithMessage("*does/not/exist.bin*32*");
    }

    [Fact]
    public void Constructor_throws_when_kek_wrong_size()
    {
        var badPath = Path.Combine(_tmpDir, "bad.bin");
        File.WriteAllBytes(badPath, new byte[16]);
        var act = () => new FileGitLabKeyProvider(Options.Create(new GitLabOptions
        {
            KeyProvider = new GitLabOptions.KeyProviderConfig
            {
                Mode = "file",
                File = new GitLabOptions.FileConfig { KekPath = badPath },
            },
        }));
        act.Should().Throw<InvalidOperationException>()
            .WithMessage("*16 bytes*32 bytes*");
    }

    [Fact]
    public async Task Decrypt_with_wrong_kid_throws_GitLabPatDecryptException()
    {
        var p = Create();
        var (ciphertext, _) = await p.EncryptAsync(new byte[] { 1, 2, 3 });
        var act = () => p.DecryptAsync(ciphertext, "some-other-kid");
        var ex = (await act.Should().ThrowAsync<GitLabPatDecryptException>()).Which;
        ex.Message.Should().Be("GitLab PAT decryption failed (provider=file)");
    }

    [Fact]
    public async Task Decrypt_with_tampered_ciphertext_throws_without_leaking_key_bytes()
    {
        var p = Create();
        var (blob, kid) = await p.EncryptAsync(System.Text.Encoding.UTF8.GetBytes("glpat-secret"));
        blob[^1] ^= 0xFF;  // flip a tag bit → GCM authentication failure
        var act = () => p.DecryptAsync(blob, kid);
        var ex = (await act.Should().ThrowAsync<GitLabPatDecryptException>()).Which;
        ex.Message.Should().NotContain("kek");
        ex.Message.Should().NotContain(_kekPath);
        ex.Message.Should().Be("GitLab PAT decryption failed (provider=file)");
    }
}

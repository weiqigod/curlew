using ApiTool.Backend;
using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests;

public sealed class DevCommandsTests
{
    [Fact]
    public void Run_empty_args_returns_null_allowing_web_host_to_start()
        => DevCommands.Run([]).Should().BeNull();

    [Fact]
    public void Run_non_dev_first_arg_returns_null()
        => DevCommands.Run(["run"]).Should().BeNull();

    [Fact]
    public void Run_dev_without_subcommand_returns_exit_2()
        => DevCommands.Run(["dev"]).Should().Be(2);

    [Fact]
    public void Run_dev_unknown_subcommand_returns_exit_2()
        => DevCommands.Run(["dev", "unknown-command"]).Should().Be(2);

    [Fact]
    public void Run_dev_email_preview_without_slug_returns_exit_2()
        => DevCommands.Run(["dev", "email-preview"]).Should().Be(2);

    [Fact]
    public void Run_dev_upload_templates_without_api_key_returns_exit_2()
        => DevCommands.Run(["dev", "upload-templates"]).Should().Be(2);

    /// <summary>
    /// Exercises the full DevCommands dispatch path for email-preview:
    /// template root resolution → MJML compile → stdout write → exit 0.
    /// </summary>
    [Fact]
    public void Run_dev_email_preview_happy_path_writes_html_to_stdout_and_returns_exit_0()
    {
        // Capture stdout without changing the real Console.Out permanently.
        var captured = new StringWriter();
        var original = Console.Out;
        Console.SetOut(captured);
        try
        {
            // Change CWD to repo root so ResolveTemplatesRoot finds templates/email.
            var repoRoot = FindRepoRoot();
            var savedDir = Directory.GetCurrentDirectory();
            Directory.SetCurrentDirectory(repoRoot);
            try
            {
                var exitCode = DevCommands.Run(["dev", "email-preview", "email_verification"]);
                exitCode.Should().Be(0, because: "valid slug should render and exit cleanly");
            }
            finally
            {
                Directory.SetCurrentDirectory(savedDir);
            }
        }
        finally
        {
            Console.SetOut(original);
        }

        var html = captured.ToString();
        html.Length.Should().BeGreaterThan(1000, because: "rendered MJML should produce a full HTML document");
        html.Should().Contain("<html", because: "output should be a valid HTML document");
    }

    /// <summary>
    /// Verifies that SENDGRID_API_BASE overrides the hard-coded production URL,
    /// enabling CI to point upload-templates at a localhost fake SendGrid.
    /// </summary>
    [Fact]
    public async Task UploadTemplates_uses_SENDGRID_API_BASE_env_var_when_set()
    {
        // Spin up a minimal fake SendGrid HTTP listener on a high port.
        const string prefix = "http://localhost:18082/";
        using var listener = new System.Net.HttpListener();
        listener.Prefixes.Add(prefix);
        listener.Start();

        // The fake responds to all requests with SendGrid-shaped JSON.
        var serverTask = Task.Run(async () =>
        {
            var idSeq = 1;
            while (listener.IsListening)
            {
                System.Net.HttpListenerContext? ctx;
                try { ctx = await listener.GetContextAsync(); }
                catch { break; }

                var path = ctx.Request.Url?.AbsolutePath ?? "";
                string responseBody;
                int statusCode;
                if (ctx.Request.HttpMethod == "GET" && path.StartsWith("/v3/templates"))
                {
                    responseBody = """{"templates":[]}""";
                    statusCode = 200;
                }
                else if (ctx.Request.HttpMethod == "POST" && path == "/v3/templates")
                {
                    responseBody = $$"""{"id":"d-fake-{{idSeq++:000}}"}""";
                    statusCode = 201;
                }
                else if (ctx.Request.HttpMethod == "POST" && path.Contains("/versions"))
                {
                    responseBody = """{"id":"v-1","active":1}""";
                    statusCode = 201;
                }
                else
                {
                    responseBody = """{"error":"not found"}""";
                    statusCode = 404;
                }

                ctx.Response.StatusCode = statusCode;
                ctx.Response.ContentType = "application/json";
                var bytes = System.Text.Encoding.UTF8.GetBytes(responseBody);
                ctx.Response.ContentLength64 = bytes.Length;
                await ctx.Response.OutputStream.WriteAsync(bytes);
                ctx.Response.Close();
            }
        });

        // Give the server a moment to start listening.
        await Task.Delay(50);

        var repoRoot = FindRepoRoot();
        var savedDir = Directory.GetCurrentDirectory();
        Directory.SetCurrentDirectory(repoRoot);

        var captured = new StringWriter();
        var originalOut = Console.Out;
        Console.SetOut(captured);

        try
        {
            Environment.SetEnvironmentVariable("SENDGRID_API_BASE", "http://localhost:18082");
            try
            {
                var exitCode = DevCommands.Run(["dev", "upload-templates", "--api-key", "fake-ci-key"]);
                exitCode.Should().Be(0, because: "upload-templates should succeed against fake SendGrid");
            }
            finally
            {
                Environment.SetEnvironmentVariable("SENDGRID_API_BASE", null);
            }
        }
        finally
        {
            Console.SetOut(originalOut);
            Directory.SetCurrentDirectory(savedDir);
            listener.Stop();
        }

        var output = captured.ToString();
        output.Should().Contain("APITOOL__SENDGRID__TEMPLATES__",
            because: "upload-templates must emit env-var lines for each uploaded slug");

        // Count emitted lines; we expect exactly one per slug in the canonical M14 inventory.
        var lines = output.Split('\n', StringSplitOptions.RemoveEmptyEntries);
        lines.Count(l => l.StartsWith("APITOOL__SENDGRID__TEMPLATES__")).Should().Be(EmailTemplateInventory.Slugs.Count,
            because: "exactly one env-var line per canonical M14 template slug must be written — no more, no less");
    }

    private static string FindRepoRoot()
    {
        var dir = new DirectoryInfo(AppContext.BaseDirectory);
        while (dir is not null)
        {
            if (Directory.Exists(Path.Combine(dir.FullName, "templates", "email")))
                return dir.FullName;
            dir = dir.Parent!;
        }
        throw new InvalidOperationException("Cannot find repo root with templates/email directory.");
    }
}

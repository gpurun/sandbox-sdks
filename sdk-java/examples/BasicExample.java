package ai.gpurun.sandbox.examples;

import ai.gpurun.sandbox.core.Sandbox;
import ai.gpurun.sandbox.core.CommandResult;
import ai.gpurun.sandbox.core.Gpurun;

/**
 * Basic example demonstrating Gpurun Sandbox usage.
 */
public class BasicExample {
    public static void main(String[] args) {
        try (Gpurun client = Gpurun.create()) {
            // Create a sandbox
            Sandbox sandbox = client.createSandbox("base");
            System.out.println("Created sandbox: " + sandbox.getId());

            try {
                // Write a file
                String content = "Hello, World from Java SDK!";
                sandbox.files().write("/tmp/hello.txt", content.getBytes(), null);
                System.out.println("Wrote file to sandbox");

                // Run a command
                CommandResult result = sandbox.commands().run("cat /tmp/hello.txt", null);
                System.out.println("Command output: " + result.getStdout());

            } finally {
                // Always clean up
                sandbox.kill();
                System.out.println("Sandbox terminated");
            }

        } catch (Exception e) {
            System.err.println("Error: " + e.getMessage());
            e.printStackTrace();
        }
    }
}

package ai.gpurun.sandbox.boxd;

import com.google.gson.JsonObject;
import com.google.gson.annotations.SerializedName;

import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Map;

public final class BoxdModels {
    private BoxdModels() {
    }

    public static class BoxHealth {
        public String status;
        public String service;
        public long startedAtUnixNano;
    }

    public static class ExecResult {
        public int exitCode;
        public String stdout = "";
        public String stderr = "";
    }

    public static class ProcessInfo {
        public int pid;
        public String tag;
        public List<String> cmd;
        public List<String> args;
        public String cwd;
        public boolean tty;
        public int rows;
        public int cols;
    }

    public static class StartProcessRequest {
        public List<String> cmd = new ArrayList<>();
        public String cwd;
        public Map<String, String> env;
        public boolean tty;
        public int rows;
        public int cols;
        public String tag;
    }

    public static class ProcessExit {
        public int exitCode;
        public boolean exited;
        public String status;
        public String error;
    }

    public static class ProcessStreamConfig {
        public String cmd;
        public List<String> args;
        public Map<String, String> envs;
        public String cwd;
    }

    public static class ProcessSize {
        public int cols;
        public int rows;
    }

    public static class ProcessSelector {
        public Integer pid;
        public String tag;

        public ProcessSelector() { }
        public ProcessSelector(int pid) { this.pid = pid; }
        public ProcessSelector(String tag) { this.tag = tag; }
    }

    public static class ConnectProcessInfo {
        public ProcessStreamConfig config;
        public int pid;
        public String tag;
    }

    public static class ProcessPTY {
        public ProcessSize size;
    }

    public static class StartProcessConnectRequest {
        public ProcessStreamConfig process;
        public ProcessPTY pty;
        public String tag;
        public boolean stdin;
    }

    public static class StartProcessStreamRequest extends StartProcessConnectRequest {
    }

    public static class ConnectRequest {
        public ProcessSelector process;
    }

    public static class ConnectProcessStreamRequest extends ConnectRequest {
    }

    public static class UpdateProcessRequest {
        public ProcessSelector process;
        public ProcessPTY pty;
    }

    public static class SendInputRequest {
        public ProcessSelector process;
        public SendInputPayload input;
    }

    public static class SendInputPayload {
        public String stdin;
        public String pty;
    }

    public static class SendSignalRequest {
        public ProcessSelector process;
        public int signal;
    }

    public static class ProcessStartEvent {
        public int pid;
    }

    /**
     * One chunk of Connect-stream process output. Fields are standard base64
     * strings because the Connect envelope is JSON and cannot carry raw
     * bytes; decode before use, or call the decode helpers.
     */
    public static class ProcessDataEvent {
        public String stdout;
        public String stderr;
        public String pty;

        /** Base64-decodes stdout; empty array when the field is absent. */
        public byte[] decodeStdout() { return decodeField(stdout); }

        /** Base64-decodes stderr; empty array when the field is absent. */
        public byte[] decodeStderr() { return decodeField(stderr); }

        /** Base64-decodes pty; empty array when the field is absent. */
        public byte[] decodePty() { return decodeField(pty); }

        private static byte[] decodeField(String value) {
            if (value == null || value.isEmpty()) return new byte[0];
            try {
                return Base64.getDecoder().decode(value);
            } catch (IllegalArgumentException e) {
                throw new BoxdException(502, "invalid_response",
                        "invalid base64 in process data event", e);
            }
        }
    }

    public static class ProcessEventBody {
        public ProcessStartEvent start;
        public ProcessDataEvent data;
        public Object keepalive;
        public ProcessExit end;
    }

    public static class ProcessEvent {
        @SerializedName("event")
        public ProcessEventBody event;
    }

    /** A flat frame from the process WebSocket connect endpoint (type/process/exit). */
    public static class ProcessWSEvent {
        public String type;
        public JsonObject process;
        public ProcessExit exit;
    }
}

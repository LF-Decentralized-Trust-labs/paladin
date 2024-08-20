import org.gradle.api.DefaultTask
import org.gradle.api.file.FileTree
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputFiles
import org.gradle.api.tasks.Nested
import org.gradle.api.tasks.Optional
import org.gradle.api.tasks.OutputFiles
import org.gradle.api.tasks.TaskAction
import org.gradle.process.ExecSpec

class ProtoCompile extends DefaultTask {

    private final String protoc = 'protoc'

    @Input
    @Optional
    String protocPath

    @Input
    List<String> protoPaths = []

    @InputFiles
    Set<File> protoFiles = []

    @Input
    List<String> args = []

    @Nested
    Plugins plugins = new Plugins()

    void protocPath(Object path) {
        protocPath = project.file(path)
    }

    void protoPath(Object path) {
        protoPaths << project.file(path)
    }

    void protoFiles(Object... paths) {
        protoFiles += project.files(paths).files
    }

    void args(Object... args) {
        this.args += [*args]
    }

    void plugins(Closure c) {
        c.delegate = plugins
        c.resolveStrategy = Closure.DELEGATE_FIRST
        c(c.delegate)
    }

    @TaskAction
    void exec() {
        String path
        if (protocPath != null) {
            path = protocPath + File.pathSeparator + System.getenv('PATH')
        }

        List<String> cmd = [protoc, *args]
        protoPaths.each { p -> cmd << "--proto_path=${p}" }
        protoFiles.each { f -> cmd << f }

        Plugins plugins = this.plugins

        project.exec { spec ->
            commandLine cmd
            if (path != null) {
                environment 'PATH', path
            }
            plugins.configure spec
        }
    }

    class Plugins {

        private Plugin go
        private Plugin go_grpc

        @Nested
        Plugin getGo() {
            if (go == null) {
                go = new Plugin('go', '**/*.pb.go')
            }
            return go
        }

        @Nested
        Plugin getGo_grpc() {
            if (go_grpc == null) {
                go_grpc = new Plugin('go-grpc')
            }
            return go_grpc
        }

        void go(Closure c) {
            c.delegate = getGo()
            c.resolveStrategy = Closure.DELEGATE_FIRST
            c(c.delegate)
        }

        void go_grpc(Closure c) {
            c.delegate = getGo_grpc()
            c.resolveStrategy = Closure.DELEGATE_FIRST
            c(c.delegate)
        }

        protected void configure(ExecSpec spec) {
            if (go != null) {
                go.configure spec
            }
            if (go_grpc != null) {
                go_grpc.configure spec
            }
        }

        class Plugin {

            private final String prefix
            private final String filePattern

            @Input
            Object out

            @Input
            List<String> opts = []

            @OutputFiles
            @Optional
            FileTree getOutputFiles() {
                if (filePattern == null) {
                    return null
                }
                return project.fileTree(out) {
                    include filePattern
                }
            }

            Plugin(String prefix, String filePattern = null) {
                this.prefix = prefix
                this.filePattern = filePattern
            }

            void out(Object out) {
                this.out = out
            }

            void opt(Object opt) {
                opts << opt
            }

            protected void configure(ExecSpec spec) {
                if (out != null) {
                    spec.args "--${prefix}_out=${out}"
                }
                opts.each { o -> spec.args "--${prefix}_opt=${o}" }
            }

        }

    }

}

import org.gradle.api.DefaultTask
import org.gradle.process.ExecSpec

class ProtoCompile extends DefaultTask {

    private String _protoc = 'protoc'
    private String _protocPath = null
    private List<String> _protoPaths = []
    private Set<File> _protoFiles = []
    private List<String> _args = []
    private Plugins _plugins = new Plugins()

    class Plugins {

        private Plugin _go
        private Plugin _go_grpc

        class Plugin {

            private String _prefix
            private String _filePattern
            private Object _out
            private List<String> _opts = []

            Plugin(String prefix, String filePattern = null) {
                _prefix = prefix
                _filePattern = filePattern
            }

            void out(Object out) {
                _out = out
                if (_filePattern != null) {
                    outputs.files project.fileTree(out) {
                        include _filePattern
                    }
                }
            }

            void opt(Object opt) {
                _opts << opt
            }

            protected void configure(ExecSpec spec) {
                if (_out != null) {
                    spec.args "--${_prefix}_out=${_out}"
                }
                _opts.each { o -> spec.args "--${_prefix}_opt=${o}" }
            }

        }

        void go(Closure c) {
            _go = new Plugin('go', '**/*.pb.go')
            c.delegate = _go
            c.resolveStrategy = Closure.DELEGATE_FIRST
            c(_go)
        }

        void go_grpc(Closure c) {
            _go_grpc = new Plugin('go-grpc')
            c.delegate = _go_grpc
            c.resolveStrategy = Closure.DELEGATE_FIRST
            c(_go_grpc)
        }

        protected void configure(ExecSpec spec) {
            if (_go != null) {
                _go.configure spec
            }
            if (_go_grpc != null) {
                _go_grpc.configure spec
            }
        }

    }

    ProtoCompile() {
        doFirst {
            this.exec()
        }
    }

    void protocPath(Object path) {
        _protocPath = project.file(path)
    }

    void protoPath(Object path) {
        _protoPaths << project.file(path)
    }

    void protoFiles(Object... paths) {
        _protoFiles += project.files(paths).files
        inputs.files(paths)
    }

    void args(Object... args) {
        _args += [*args]
    }

    void plugins(Closure c) {
        c.delegate = _plugins
        c.resolveStrategy = Closure.DELEGATE_FIRST
        c(_plugins)
    }

    protected void exec() {
        String path
        if (_protocPath != null) {
            path = _protocPath + File.pathSeparator + System.getenv('PATH')
        }

        List<String> cmd = [_protoc, *_args]
        _protoPaths.each { p -> cmd << "--proto_path=${p}" }
        _protoFiles.each { f -> cmd << f }

        Plugins plugins = _plugins

        project.exec { spec ->
            commandLine cmd
            if (path != null) {
                environment 'PATH', path
            }
            plugins.configure spec
        }
    }

}

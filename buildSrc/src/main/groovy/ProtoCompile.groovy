import org.gradle.api.DefaultTask

class ProtoCompile extends DefaultTask {

    private String _protoc = "protoc"
    private String _protocPath = null
    private List<String> _protoPaths = []
    private Set<File> _protoFiles = []
    private List<String> _args = []

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

    protected void exec() {
        String path
        if (_protocPath != null) {
            path = _protocPath + File.pathSeparator + System.getenv('PATH')
        }

        List<String> cmd = [_protoc, *_args]
        _protoPaths.each { p -> cmd << "--proto_path=${p}" }
        _protoFiles.each { f -> cmd << f }

        project.exec {
            commandLine cmd
            if (path != null) {
                environment 'PATH', path
            }
        }
    }

}

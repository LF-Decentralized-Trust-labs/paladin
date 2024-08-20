import org.gradle.api.DefaultTask
import org.gradle.process.ExecSpec

class Mockery extends DefaultTask {

    private Object _mockery = 'mockery'
    private List<Mock> _mocks = []
    private List<String> _commonArgs = []

    Mockery() {
        doFirst {
            this.exec()
        }
    }

    void mockery(Object m) {
        _mockery = m
        inputs.file(m)
    }

    void mock(Closure c) {
        Mock m = new Mock()
        c.delegate = m
        c.resolveStrategy = Closure.DELEGATE_FIRST
        c(m)
        _mocks << m
    }

    void args(Object... args) {
        _commonArgs += [*args]
    }

    protected void exec() {
        List<String> commonArgs = _commonArgs
        String mockery = _mockery
        _mocks.each { m ->
            project.exec { spec ->
                executable mockery
                args commonArgs
                m.configure spec
            }
        }
    }

    class Mock {

        private String _inputDir
        private String _name
        private boolean _includeAll = false
        private String _outputPackage
        private String _outputDir

        void inputDir(String dir) {
            _inputDir = dir
            inputs.dir dir
        }

        void name(String name) {
            _name = name
        }

        void includeAll(boolean include) {
            _includeAll = include
        }

        void outputPackage(String outpkg) {
            _outputPackage = outpkg
        }

        void outputDir(String output) {
            _outputDir = output
            outputs.dir output
        }

        protected void configure(ExecSpec spec) {
            if (_inputDir != null) {
                spec.args '--dir', _inputDir
            }
            if (_name != null) {
                spec.args '--name', _name
            }
            if (_includeAll) {
                spec.args '--all'
            }
            if (_outputPackage != null) {
                spec.args '--outpkg', _outputPackage
            }
            if (_outputDir != null) {
                spec.args '--output', _outputDir
            }
        }

    }

}

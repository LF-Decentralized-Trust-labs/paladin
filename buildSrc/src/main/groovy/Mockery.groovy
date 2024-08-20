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

        private String _dir
        private String _name
        private boolean _includeAll = false
        private String _outpkg
        private String _output

        void dir(String dir) {
            _dir = dir
        }

        void name(String name) {
            _name = name
        }

        void includeAll(boolean include) {
            _includeAll = include
        }

        void outpkg(String outpkg) {
            _outpkg = outpkg
        }

        void output(String output) {
            _output = output
            outputs.dir output
        }

        protected void configure(ExecSpec spec) {
            if (_dir != null) {
                spec.args '--dir', _dir
            }
            if (_name != null) {
                spec.args '--name', _name
            }
            if (_includeAll) {
                spec.args '--all'
            }
            if (_outpkg != null) {
                spec.args '--outpkg', _outpkg
            }
            if (_output != null) {
                spec.args '--output', _output
            }
        }

    }

}

import org.gradle.api.DefaultTask
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputDirectory
import org.gradle.api.tasks.Nested
import org.gradle.api.tasks.Optional
import org.gradle.api.tasks.OutputDirectory
import org.gradle.api.tasks.TaskAction
import org.gradle.process.ExecSpec

class Mockery extends DefaultTask {

    private String mockery = 'mockery'

    @Nested
    List<Mock> mocks = []

    @Input
    List<String> args = []

    void mockery(Object m) {
        mockery = project.file(m)
        inputs.file(m)
    }

    void mock(Closure c) {
        Mock m = new Mock()
        c.delegate = m
        c.resolveStrategy = Closure.DELEGATE_FIRST
        c(m)
        mocks << m
    }

    void args(Object... args) {
        this.args += [*args]
    }

    @TaskAction
    void exec() {
        List<String> commonArgs = args
        String mockery = this.mockery
        mocks.each { m ->
            project.exec { spec ->
                executable mockery
                args commonArgs
                m.configure spec
            }
        }
    }

    class Mock {

        @InputDirectory
        File inputDir

        @Input
        @Optional
        String name

        @Input
        boolean includeAll = false

        @Input
        @Optional
        String outputPackage

        @OutputDirectory
        File outputDir

        void inputDir(Object dir) {
            inputDir = project.file(dir)
        }

        void name(String name) {
            this.name = name
        }

        void includeAll(boolean include) {
            includeAll = include
        }

        void outputPackage(String outpkg) {
            outputPackage = outpkg
        }

        void outputDir(Object output) {
            outputDir = project.file(output)
        }

        protected void configure(ExecSpec spec) {
            if (inputDir != null) {
                spec.args '--dir', inputDir
            }
            if (name != null) {
                spec.args '--name', name
            }
            if (includeAll) {
                spec.args '--all'
            }
            if (outputPackage != null) {
                spec.args '--outpkg', outputPackage
            }
            if (outputDir != null) {
                spec.args '--output', outputDir
            }
        }

    }

}

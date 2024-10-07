import org.apache.tools.ant.taskdefs.condition.Os
import org.gradle.api.DefaultTask
import org.gradle.api.file.FileCollection
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputFiles
import org.gradle.api.tasks.Internal
import org.gradle.api.tasks.TaskAction
import org.gradle.process.ExecResult

class GoLib extends DefaultTask {

    @Input
    String baseName

    @Input
    String mainFile

    @InputFiles
    FileCollection sources

    @Internal
    String outputDir

    @Internal
    String outputLib

    @Internal
    String outputHeader

    void sources(Object... sources) {
        if (this.sources == null) {
            this.sources = project.files(sources)
        } else {
            this.sources += project.files(sources)
        }
    }

    void baseName(String baseName) {
        this.baseName = baseName

        def libName;
        if (Os.isFamily(Os.FAMILY_WINDOWS)) {
            libName = "lib${baseName}.dll"
        } else if (Os.isFamily(Os.FAMILY_MAC)) {
            libName = "lib${baseName}.dylib"
        } else {
            libName = "lib${baseName}.so"
        }

        // Updated paths for outputs
        outputDir = "${getProject().layout.buildDirectory.dir("libs").get().asFile.getAbsolutePath()}"
        outputLib = "${outputDir}/${libName}"
        outputHeader = "${outputDir}/lib${libName}.h"

        outputs.files(outputLib, outputHeader)
    }

    void mainFile(String mainFile) {
        this.mainFile = mainFile
    }

    @TaskAction
    void exec() {

        def cmd = [
            'go', 'build',
            '-o', outputLib,
            '-buildmode=c-shared',
            "${mainFile}"
        ]

        ExecResult execResult = project.exec {
            commandLine cmd
            environment("CGO_ENABLED", "1")
        }
        if (execResult.exitValue != 0) {
            println "\nGo build failed: '${cmd}'"
        }
        execResult.assertNormalExitValue()
    }

}

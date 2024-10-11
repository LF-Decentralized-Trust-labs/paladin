import org.gradle.api.DefaultTask
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.OutputDirectory
import org.gradle.api.tasks.Optional
import org.gradle.api.tasks.TaskAction
import org.gradle.process.ExecSpec
import java.io.FileOutputStream

abstract class GoTestRunner extends DefaultTask {

    @Input
    List<String> testArgs = []

    @OutputDirectory
    File coverOutputDirectory = project.file("coverage")

    @OutputDirectory
    File resultsOutputDirectory = project.file("results")

    @Input
    @Optional
    String resultsOutputFile = ""

    @Input
    String testTimeout = "30s"

    @Input
    boolean verbose = project.properties.get("verboseTests") == "true"

    @Input
    @Optional
    String coverMode = "atomic"

    @Input
    boolean ciMode = project.properties.get("ci") == "true"

    @Input
    @Optional
    String coverPkg = ""

    void addTestArgs(String... args) {
        testArgs.addAll(args)
    }

    void setCoverOutputDirectory(File outputDir) {
        this.coverOutputDirectory = outputDir
    }

    void setResultsOutputDirectory(File outputDir) {
        this.resultsOutputDirectory = outputDir
    }

    void setResultsOutputFile(String resultsOutputFile) {
        this.resultsOutputFile = resultsOutputFile
    }

    void setTestTimeout(String timeout) {
        this.testTimeout = timeout
    }

    void setVerbose(boolean verbose) {
        this.verbose = verbose
    }

    void setCIMode(boolean ciMode) {
        this.ciMode = ciMode
    }

    void setCoverMode(String coverMode) {
        this.coverMode = coverMode
    }

    void setCoverPkg(String coverPkg) {
        this.coverPkg = coverPkg
    }
     
    @TaskAction
    void exec() {

        project.exec({ ExecSpec execSpec ->
            execSpec.workingDir = project.projectDir
            execSpec.executable = "go"
            execSpec.args("test")
            execSpec.args(testArgs)
            execSpec.args("-cover")
            execSpec.args("-covermode=" + coverMode)
            if (coverPkg) {
                execSpec.args("-coverpkg=" + coverPkg)
            }
            execSpec.args("-timeout=" + testTimeout)
            if (verbose) {
                execSpec.args("-v")
            }
            if (ciMode) {
                // execSpec.args("-json")
                def outputFile = new File(resultsOutputDirectory, resultsOutputFile)
                execSpec.standardOutput = new FileOutputStream(outputFile)

            }
            if (coverOutputDirectory) {
                execSpec.args("-test.gocoverdir=" + coverOutputDirectory.absolutePath)
            }
        })
    }
}
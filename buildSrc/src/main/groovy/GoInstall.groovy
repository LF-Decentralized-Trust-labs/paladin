
import org.gradle.api.DefaultTask
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.OutputDirectory
import org.gradle.api.tasks.TaskAction
import org.gradle.process.ExecResult

class GoInstall extends DefaultTask {

    @Input
    String tool

    @Input
    String toolVersion

    @OutputDirectory
    File outputDir = project.file(["go", "env", "GOMODCACHE"].execute().text)

    @TaskAction
    void exec() {
        List<String> cmd = ["go", "install", "${tool}@${toolVersion}"]
        ExecResult execResult = project.exec { commandLine cmd }
        if (execResult.exitValue != 0) {
            println "\nGo install failed: '${cmd}'"
        }
        execResult.assertNormalExitValue()
    }

}

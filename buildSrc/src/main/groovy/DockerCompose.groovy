import org.gradle.api.DefaultTask
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputFiles
import org.gradle.api.tasks.Optional
import org.gradle.api.tasks.TaskAction
import org.gradle.process.ExecResult

class DockerCompose extends DefaultTask {

    @InputFiles
    List<File> composeFiles = []

    @Input
    @Optional
    String projectName

    @Input
    List<String> args = []

    void composeFile(Object f) {
        composeFiles << project.file(f)
    }

    void projectName(String p) {
        projectName = p
    }

    void args(Object... args) {
        this.args += [*args]
    }

    void dumpLogs(String service = '') {
        List<String> cmd = [*dockerCommand(), 'logs']
        if (service == '') {
            println 'Dumping Docker logs'
        } else {
            println "Dumping Docker logs for ${service}"
            cmd << service
        }
        project.exec { commandLine cmd }
    }

    @TaskAction
    void exec() {
        List<String> cmd = [*dockerCommand(), *args]
        ExecResult execResult = project.exec { commandLine cmd }
        if (execResult.exitValue != 0) {
            dumpLogs()
        }
        execResult.assertNormalExitValue()
    }

    private List<String> dockerCommand() {
        String dockerComposeV2Check = 'docker compose version'.execute().text
        List<String> cmd = dockerComposeV2Check.contains('Docker Compose')
            ? ['docker', 'compose'] : ['docker-compose']
        composeFiles.each { f ->
            cmd += ['-f', f]
        }
        if (projectName != null) {
            cmd += ['-p', projectName]
        }
        return cmd
    }

}

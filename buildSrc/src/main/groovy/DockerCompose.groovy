import org.gradle.process.ExecResult
import org.gradle.api.DefaultTask

class DockerCompose extends DefaultTask {

    private List<String> _composeFiles = []
    private List<String> _args = []

    DockerCompose() {
        doFirst {
            this.exec()
        }
    }

    void composeFile(String f) {
        _composeFiles += ['-f', f]
    }

    void args(Object... args) {
        _args += [*args]
    }

    void dumpLogs(String service = "") {
        if (service == "") {
            println "Dumping Docker logs"
        } else {
            println "Dumping Docker logs for ${service}"
        }
        "${dockerCommand().join(' ')} logs ${service}"
            .execute().waitForProcessOutput(System.err, System.err)
    }

    private List<String> dockerCommand() {
        if (_composeFiles.size() == 0) {
            _composeFiles = ['-f', 'docker-compose.yml']
        }
        String dockerComposeV2Check = 'docker compose version'.execute().text
        if (dockerComposeV2Check.contains('Docker Compose')) {
            return ['docker', 'compose', *_composeFiles]
        }
        return ['docker-compose', *_composeFiles]
    }

    protected void exec() {
        List<String> cmd = [*dockerCommand(), *_args]
        ExecResult execResult = project.exec {
            executable cmd.remove(0)
            args cmd
        }
        if (execResult.exitValue != 0) {
            dumpLogs()
        }
        execResult.assertNormalExitValue()
    }

}

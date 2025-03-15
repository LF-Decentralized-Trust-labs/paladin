import org.gradle.api.DefaultTask
import org.gradle.api.GradleException
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputFile
import org.gradle.api.tasks.TaskAction

class GoCheckCoverage extends DefaultTask {

    @InputFile
    File coverageFile

    @Input
    BigDecimal target

    @Input
    BigDecimal maxGap

    void coverageFile(Object f) {
        coverageFile = project.file(f)
    }

    @TaskAction
    void exec() {
        String coverageOutput = ("go tool cover -func=${coverageFile}").execute().text
        String totalCoverage = coverageOutput.readLines().find { line ->
            line.contains('total:')
        }?.split()?.last()?.replace('%', '')
        println "Coverage is ${totalCoverage}%"
        if (totalCoverage && totalCoverage.toFloat() < target) {
            throw new GradleException(
                "ERROR: Coverage is below ${target}% (current coverage: ${totalCoverage}%)"
            )
        } else if (totalCoverage.toFloat() - target > maxGap) {
            throw new GradleException(
                "ERROR: The target coverage ${target}% is below the current coverage: ${totalCoverage}% by " +
                "more than ${maxGap}%; please update the target value in build.gradle."
            )
        } else {
            println "Coverage is above ${target}%, current coverage: ${totalCoverage}%"
        }
    }

}
